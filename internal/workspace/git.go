// Package workspace groups the repository-facing operations: git helpers, worktrees, local changes and
// snapshots, editors and dev servers.
package workspace

import (
	"infuse/internal/platform/process"
	"os/exec"
	"strings"
)

// GitRun runs git in root and returns its combined output.
func GitRun(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// GitOutput runs git in root and returns its trimmed stdout ("" on failure).
func GitOutput(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
