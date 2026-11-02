//go:build windows

package config

import (
	"infuse/internal/core"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// vaultIsOurs reports whether this process runs on the user's real infuse home. Credential Manager is
// per Windows user, not per folder: redirecting USERPROFILE or INFUSE_HOME does not isolate it, so a
// test or a second instance on another home that reached it would read the user's real keys and,
// through the migration, take them out of the vault for good. The profile comes from the token, which
// the environment cannot change.
func vaultIsOurs() bool {
	profile, err := windows.GetCurrentProcessToken().GetUserProfileDirectory()
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(core.InfuseHome()), filepath.Join(profile, core.InfuseDirName))
}
