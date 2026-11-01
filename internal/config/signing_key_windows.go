//go:build windows

package config

// Windows keeps the fallback signing key inside a DPAPI blob, so the bytes on disk only decode for
// the same Windows user on the same machine. A copied or stolen key file no longer signs anything.

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

var (
	modcrypt32            = windows.NewLazySystemDLL("crypt32.dll")
	procCryptProtectData  = modcrypt32.NewProc("CryptProtectData")
	procCryptUnprotectDat = modcrypt32.NewProc("CryptUnprotectData")
)

const cryptUIForbidden = 0x1

var keyEntropy = []byte("infuse-signing-v1")

type dataBlob struct {
	Size uint32
	Data *byte
}

func blobOf(b []byte) dataBlob {
	if len(b) == 0 {
		return dataBlob{}
	}
	return dataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func protectKey(k []byte) (blob []byte, protected bool, err error) {
	in := blobOf(k)
	entropy := blobOf(keyEntropy)
	var out dataBlob
	r1, _, _ := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&in)), 0, uintptr(unsafe.Pointer(&entropy)), 0, 0, cryptUIForbidden,
		uintptr(unsafe.Pointer(&out)))
	if r1 == 0 {
		return nil, false, errors.New("CryptProtectData failed")
	}
	return localBlob(out), true, nil
}

func unprotectKeyBytes(blob []byte) ([]byte, error) {
	in := blobOf(blob)
	entropy := blobOf(keyEntropy)
	var out dataBlob
	r1, _, _ := procCryptUnprotectDat.Call(
		uintptr(unsafe.Pointer(&in)), 0,
		uintptr(unsafe.Pointer(&entropy)), 0, 0, cryptUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r1 == 0 {
		return nil, errors.New("CryptUnprotectData failed")
	}
	return localBlob(out), nil
}

// localBlob copies the API-owned buffer before it is released: the returned slice must own its bytes.
func localBlob(out dataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	if out.Size == 0 || out.Data == nil {
		return nil
	}
	b := make([]byte, out.Size)
	copy(b, unsafe.Slice(out.Data, out.Size))
	return b
}
