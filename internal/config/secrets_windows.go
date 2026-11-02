//go:build windows

package config

// Windows Credential Manager, read only to migrate what older builds stored there: it answers a Low
// integrity process of the same user, so secrets now live in secrets_file_windows.go.

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredReadW  = advapi32.NewProc("CredReadW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

type credentialW struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

const (
	credTypeGeneric = 1
	errorNotFound   = syscall.Errno(1168)
)

func credGet(name string) (string, error) {
	target, _ := windows.UTF16PtrFromString(name)
	var pc *credentialW
	if r, _, err := procCredReadW.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0, uintptr(unsafe.Pointer(&pc))); r == 0 {
		if errors.Is(err, errorNotFound) {
			return "", nil
		}
		return "", err
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(pc)))
	if pc.CredentialBlobSize == 0 || pc.CredentialBlob == nil {
		return "", nil
	}
	return string(unsafe.Slice(pc.CredentialBlob, pc.CredentialBlobSize)), nil
}

func credDelete(name string) error {
	target, _ := windows.UTF16PtrFromString(name)
	if r, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); r == 0 && !errors.Is(err, errorNotFound) {
		return err
	}
	return nil
}
