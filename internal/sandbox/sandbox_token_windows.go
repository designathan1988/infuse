//go:build windows

package sandbox

// The contained token: the same user at Low integrity, without privileges.

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	disableMaxPriv = 0x1 // every privilege but SeChangeNotify goes away
	sandboxInert   = 0x2 // AppLocker and Software Restriction Policies do not apply twice
)

var (
	modadvapi                 = windows.NewLazySystemDLL("advapi32.dll")
	procCreateRestrictedToken = modadvapi.NewProc("CreateRestrictedToken")
)

// tokenMandatoryLabel mirrors TOKEN_MANDATORY_LABEL.
type tokenMandatoryLabel struct{ Label windows.SIDAndAttributes }

// lowToken duplicates the current token without its privileges, at Low integrity.
func lowToken() (windows.Token, error) {
	var tok windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_DUPLICATE|windows.TOKEN_QUERY|windows.TOKEN_ADJUST_DEFAULT|windows.TOKEN_ASSIGN_PRIMARY, &tok); err != nil {
		return 0, err
	}
	defer tok.Close()
	var restricted windows.Token
	r1, _, callErr := procCreateRestrictedToken.Call(uintptr(tok), disableMaxPriv|sandboxInert,
		0, 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&restricted)))
	if r1 == 0 {
		return 0, fmt.Errorf("CreateRestrictedToken: %w", callErr)
	}
	low, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		restricted.Close()
		return 0, err
	}
	info := tokenMandatoryLabel{Label: windows.SIDAndAttributes{Sid: low, Attributes: windows.SE_GROUP_INTEGRITY}}
	size := uint32(unsafe.Sizeof(info)) + windows.GetLengthSid(low)
	if err := windows.SetTokenInformation(restricted, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&info)), size); err != nil {
		restricted.Close()
		return 0, fmt.Errorf("integrity level: %w", err)
	}
	return restricted, nil
}
