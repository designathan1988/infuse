//go:build windows

package main

// Start-app enumeration: MSIX apps (ChatGPT, Claude desktop) are only visible here, not as .exe paths.

import (
	"encoding/json"
	"os/exec"
)

func startApps() []startApp {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", `Get-StartApps | ConvertTo-Json -Compress`)
	noConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}
	var many []startApp
	if json.Unmarshal(out, &many) == nil {
		return many
	}
	var one startApp // ConvertTo-Json emits one object when there is exactly one entry
	if json.Unmarshal(out, &one) == nil && one.AppID != "" {
		return []startApp{one}
	}
	return nil
}
