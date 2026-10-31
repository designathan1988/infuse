package main

// The Codex desktop app (MSIX) ships its CLI at <package>\app\resources\codex.exe. The package folder is
// versioned, so it is looked up from the package registry each time, never hard-coded.

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func bundledCodex() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", "(Get-AppxPackage -Name OpenAI.Codex | Select-Object -First 1).InstallLocation")
	noConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	loc := strings.TrimSpace(string(out))
	if loc == "" {
		return ""
	}
	p := filepath.Join(loc, "app", "resources", "codex.exe")
	if appFileExists(p) {
		return p
	}
	return ""
}
