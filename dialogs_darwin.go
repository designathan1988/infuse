//go:build darwin

package main

// macOS: NSOpenPanel, driven through AppleScript's `choose folder` / `choose file` (which present
// NSOpenPanel), so the binary needs no Objective-C glue for dialogs.

import (
	"os/exec"
	"strings"
)

type macDialogs struct{}

func (macDialogs) PickFolder(title string) (string, error) {
	out, err := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "`+asEscape(title)+`")`).Output()
	if err != nil {
		return "", nil // cancelled (osascript exits 1)
	}
	return strings.TrimRight(strings.TrimSpace(string(out)), "/"), nil
}

func (macDialogs) PickFiles(title string) ([]string, error) {
	script := `set fs to choose file with prompt "` + asEscape(title) + `" with multiple selections allowed
set out to ""
repeat with f in fs
set out to out & POSIX path of f & linefeed
end repeat
return out`
	out, err := exec.Command("osascript", "-e", script).Output()
	if err != nil {
		return nil, nil
	}
	var paths []string
	for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			paths = append(paths, l)
		}
	}
	return paths, nil
}

func osDialogs(owner func() uintptr) NativeDialogs { return macDialogs{} }
