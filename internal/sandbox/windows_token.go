//go:build windows

package sandbox

// A Windows deny-write sandbox without administrator rights: a WRITE_RESTRICTED token whose write
// access check accepts only the logon SID, Everyone and a capability SID granted on the work root.
// Reads keep the caller's identity. This is the mechanism used by current agent sandboxes on Windows
// (see docs/AUDIT-REPORT.md §9); it avoids AppContainer namespace redirection and Low integrity,
// which the MSYS2 runtime cannot start under.

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	restrictedTokenFlags = 0x1 | 0x8 // DISABLE_MAX_PRIVILEGE | WRITE_RESTRICTED
	fileAllAccess        = 0x1F01FF
)

var (
	advapi32                  = windows.NewLazySystemDLL("advapi32.dll")
	procCreateRestrictedToken = advapi32.NewProc("CreateRestrictedToken")
	procInitializeAcl         = advapi32.NewProc("InitializeAcl")
	procAddMandatoryAce       = advapi32.NewProc("AddMandatoryAce")
)

// sidMemory owns the bytes a generated SID points at; the pointer must not outlive it.
type sidMemory struct {
	sid *windows.SID
	buf []byte
}

func makeSID(authority uint64, subAuthorities ...uint32) sidMemory {
	buf := make([]byte, 8+4*len(subAuthorities))
	buf[0], buf[1] = 1, byte(len(subAuthorities))
	for i := 0; i < 6; i++ {
		buf[2+i] = byte(authority >> uint(8*(5-i)))
	}
	for i, sub := range subAuthorities {
		binary.LittleEndian.PutUint32(buf[8+4*i:], sub)
	}
	return sidMemory{sid: (*windows.SID)(unsafe.Pointer(&buf[0])), buf: buf}
}

// capabilitySID is stable for a work root, so the standing ACE is reused instead of re-propagated
// on every command.
func capabilitySID(root string) sidMemory {
	sum := sha256.Sum256([]byte(strings.ToLower(root)))
	return makeSID(4, binary.LittleEndian.Uint32(sum[0:4]), binary.LittleEndian.Uint32(sum[4:8]))
}

type restrictedToken struct {
	token  windows.Token
	capSID *windows.SID
	keep   []*windows.SID
	mem    []sidMemory
}

func (r *restrictedToken) close() {
	if r.token != 0 {
		_ = r.token.Close()
		r.token = 0
	}
}

func tokenLogonSID(token windows.Token) (*windows.SID, error) {
	groups, err := token.GetTokenGroups()
	if err != nil {
		return nil, err
	}
	for _, group := range groups.AllGroups() {
		if uint32(group.Attributes)&windows.SE_GROUP_LOGON_ID == windows.SE_GROUP_LOGON_ID {
			return group.Sid.Copy()
		}
	}
	return nil, errors.New("the token has no logon SID")
}

// newRestrictedToken builds the write-restricted primary token for one work root.
func newRestrictedToken(root string) (*restrictedToken, error) {
	access := uint32(windows.TOKEN_QUERY | windows.TOKEN_DUPLICATE | windows.TOKEN_ADJUST_DEFAULT | windows.TOKEN_ASSIGN_PRIMARY)
	var current windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), access, &current); err != nil {
		return nil, fmt.Errorf("OpenProcessToken: %w", err)
	}
	defer current.Close()
	logon, err := tokenLogonSID(current)
	if err != nil {
		return nil, err
	}
	user, err := current.GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("token user: %w", err)
	}
	world, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		return nil, fmt.Errorf("Everyone SID: %w", err)
	}
	capability := capabilitySID(root)
	// The user SID is required by the MSYS2 runtime, whose signal pipe and per-user shared memory
	// name only the user; the Low integrity label, not this SID, is what confines the file writes.
	sids := []windows.SIDAndAttributes{
		{Sid: logon},
		{Sid: world},
		{Sid: user.User.Sid},
		{Sid: capability.sid},
	}
	var out windows.Token
	r1, _, callErr := procCreateRestrictedToken.Call(
		uintptr(current), restrictedTokenFlags,
		0, 0, // no SIDs disabled
		0, 0, // no privileges deleted
		uintptr(len(sids)), uintptr(unsafe.Pointer(&sids[0])),
		uintptr(unsafe.Pointer(&out)),
	)
	if r1 == 0 {
		if callErr == nil || callErr == syscall.Errno(0) {
			callErr = errors.New("the call returned no error")
		}
		return nil, fmt.Errorf("CreateRestrictedToken: %w", callErr)
	}
	low, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		return nil, fmt.Errorf("Low integrity SID: %w", err)
	}
	label := windows.Tokenmandatorylabel{Label: windows.SIDAndAttributes{Sid: low, Attributes: windows.SE_GROUP_INTEGRITY}}
	if err := windows.SetTokenInformation(out, windows.TokenIntegrityLevel, (*byte)(unsafe.Pointer(&label)), label.Size()); err != nil {
		_ = out.Close()
		return nil, fmt.Errorf("lower token integrity: %w", err)
	}
	rt := &restrictedToken{token: out, capSID: capability.sid, keep: []*windows.SID{logon, world, user.User.Sid, low}, mem: []sidMemory{capability}}
	if err := rt.grantDefaultDACL(); err != nil {
		rt.close()
		return nil, err
	}
	return rt, nil
}

type tokenDefaultDACL struct {
	DefaultDacl *windows.ACL
}

// grantDefaultDACL lets the confined process create its own IPC objects (MSYS2's signal pipe and
// shared memory) without granting write access to existing objects.
func (r *restrictedToken) grantDefaultDACL() error {
	var size uint32
	_ = windows.GetTokenInformation(r.token, windows.TokenDefaultDacl, nil, 0, &size)
	if size == 0 {
		return errors.New("the token has no default DACL buffer")
	}
	buf := make([]byte, size)
	if err := windows.GetTokenInformation(r.token, windows.TokenDefaultDacl, &buf[0], size, &size); err != nil {
		return fmt.Errorf("read default DACL: %w", err)
	}
	current := (*tokenDefaultDACL)(unsafe.Pointer(&buf[0]))
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{allowEntry(r.capSID, fileAllAccess, 0)}, current.DefaultDacl)
	if err != nil {
		return fmt.Errorf("merge default DACL: %w", err)
	}
	updated := tokenDefaultDACL{DefaultDacl: acl}
	if err := windows.SetTokenInformation(r.token, windows.TokenDefaultDacl, (*byte)(unsafe.Pointer(&updated)), uint32(unsafe.Sizeof(updated))); err != nil {
		return fmt.Errorf("set default DACL: %w", err)
	}
	return nil
}

func allowEntry(sid *windows.SID, mask uint32, inherit uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(mask),
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inherit,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

// setLabel writes a mandatory integrity label into the SACL. An inheritable Low label on the work
// root makes the tree writable by the Low token; a protected Medium label on .infuse overrides the
// inherited label so the confined process cannot reach the records.
func setLabel(path string, label *windows.SID, inherit, protect bool) error {
	buf := make([]byte, 512)
	acl := (*windows.ACL)(unsafe.Pointer(&buf[0]))
	r1, _, err := procInitializeAcl.Call(uintptr(unsafe.Pointer(acl)), uintptr(len(buf)), 2)
	if r1 == 0 {
		return fmt.Errorf("InitializeAcl(%s): %w", path, err)
	}
	flags := uintptr(0)
	if inherit {
		flags = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	r1, _, err = procAddMandatoryAce.Call(
		uintptr(unsafe.Pointer(acl)), 2, flags, 1, // SYSTEM_MANDATORY_LABEL_NO_WRITE_UP
		uintptr(unsafe.Pointer(label)),
	)
	if r1 == 0 {
		return fmt.Errorf("AddMandatoryAce(%s): %w", path, err)
	}
	info := windows.SECURITY_INFORMATION(windows.LABEL_SECURITY_INFORMATION)
	if protect {
		info |= windows.SECURITY_INFORMATION(windows.PROTECTED_SACL_SECURITY_INFORMATION)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, nil, nil, nil, acl); err != nil {
		return fmt.Errorf("set label of %s: %w", path, err)
	}
	return nil
}

// editDACL merges one explicit entry into the ACL of a path and writes it back. Setting an
// inheritable entry on a directory makes Windows propagate it to the existing tree.
func editDACL(path string, entry windows.EXPLICIT_ACCESS) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read ACL of %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read DACL of %s: %w", path, err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry}, dacl)
	if err != nil {
		return fmt.Errorf("build ACL of %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("set ACL of %s: %w", path, err)
	}
	return nil
}

// grantWrite lets the confined token write this path and everything beneath it.
func grantWrite(path string, sid *windows.SID) error {
	return editDACL(path, allowEntry(sid, fileAllAccess, windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT))
}
