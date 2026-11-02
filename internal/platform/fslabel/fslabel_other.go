//go:build !windows

// Package fslabel marks the files only infuse itself may read. Without mandatory labels the owner-only
// mode is what the platform offers; the contained process runs as the same user, so this keeps other
// users out, not the sandbox (Landlock and Seatbelt restrict writes only).
package fslabel

import "os"

// NoReadUp restricts path to its owner.
func NoReadUp(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o600)
	if st.IsDir() {
		mode = 0o700
	}
	return os.Chmod(path, mode)
}
