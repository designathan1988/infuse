//go:build windows

// Package fslabel marks the files only infuse itself may read. It sits below config and sandbox, which
// both need it: the signing keys live in config, and the sandbox relies on the same kernel rule.
package fslabel

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// noReadUp is the Medium mandatory label with no-write-up and no-read-up: a process below Medium
// integrity can neither write nor read the object, whatever the ACLs grant the same user. Inheritable,
// so a folder passes it on to the files created inside it.
const noReadUp = "S:(ML;OICI;NWNR;;;ME)"

// NoReadUp labels path so contained (Low integrity) processes cannot read it. Idempotent.
func NoReadUp(path string) error {
	cur, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if strings.Contains(cur.String(), strings.TrimPrefix(noReadUp, "S:")) {
		return nil
	}
	sd, err := windows.SecurityDescriptorFromString(noReadUp)
	if err != nil {
		return err
	}
	sacl, _, err := sd.SACL()
	if err != nil {
		return err
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.LABEL_SECURITY_INFORMATION, nil, nil, nil, sacl); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
