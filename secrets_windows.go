//go:build windows

package main

// Windows Credential Manager (generic credentials, persisted per user on this machine).

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	advapi32       = windows.NewLazySystemDLL("advapi32.dll")
	procCredWriteW = advapi32.NewProc("CredWriteW")
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
	credTypeGeneric         = 1
	credPersistLocalMachine = 2
	errorNotFound           = syscall.Errno(1168)
)

type osSecrets struct{}

func (osSecrets) Set(name, value string) error {
	target, _ := windows.UTF16PtrFromString(name)
	user, _ := windows.UTF16PtrFromString("infuse")
	blob := []byte(value)
	c := credentialW{Type: credTypeGeneric, TargetName: target, UserName: user, Persist: credPersistLocalMachine, CredentialBlobSize: uint32(len(blob))}
	if len(blob) > 0 {
		c.CredentialBlob = &blob[0]
	}
	if r, _, err := procCredWriteW.Call(uintptr(unsafe.Pointer(&c)), 0); r == 0 {
		return err
	}
	return nil
}

func (osSecrets) Get(name string) (string, error) {
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

func (osSecrets) Delete(name string) error {
	target, _ := windows.UTF16PtrFromString(name)
	if r, _, err := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credTypeGeneric, 0); r == 0 && !errors.Is(err, errorNotFound) {
		return err
	}
	return nil
}
