package main

// The launcher: `infuse claude|codex|gemini` mounts the environment before the agent opens (SPEC §3):
// the map refreshed, the watcher running, the grounds in the file that agent reads natively, and its
// shell pointed at the interposed one, for this process only.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/knowledge"
	"infuse/internal/projectmap"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launchAgent starts an external coding agent inside the filter, without changing the user's global
// settings.
func launchAgent(kind string, args []string) int {
	exe := desktop.CLIPath(kind)
	if exe == "" {
		fmt.Printf("[infuse] %s was not found on this machine; install the CLI or use the bridge provider.\n", kind)
		return 1
	}
	if err := filter.EnsureShim(); err != nil {
		fmt.Println("[infuse] cannot install the interposed shell:", err)
		return 1
	}
	if err := filter.EnsurePowerShellShim(); err != nil {
		fmt.Println("[infuse] cannot install the interposed PowerShell:", err)
		return 1
	}
	cwd, _ := os.Getwd()
	if err := mountEnvironment(kind, core.FindRoot(cwd)); err != nil {
		fmt.Println("[infuse] cannot write the grounds:", err)
		return 1
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Dir = cwd
	cmd.Env = agentEnv(kind, os.Environ(), filter.ShimPath(), filter.PowerShellShimPath(), runtime.GOOS)
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "[infuse] cannot start", kind+":", err)
		return 1
	}
	return 0
}

// groundsFile is the instruction file each agent reads by itself, at the project's root.
var groundsFile = map[string]string{"claude": "CLAUDE.md", "codex": "AGENTS.md", "gemini": "GEMINI.md"}

// mountEnvironment prepares the project before the agent reads anything.
func mountEnvironment(kind, root string) error {
	projectmap.WriteMap(root, projectmap.BuildMap(root))
	filter.EnsureWatcher(root)
	return projectmap.UpsertBlock(filepath.Join(root, groundsFile[kind]), knowledge.BlockGrounds, knowledge.Grounds(root))
}

// agentEnv sets the variables that really decide each CLI's shell. As their sources stand:
//   - Claude Code takes the shell from CLAUDE_CODE_GIT_BASH_PATH (Windows) or CLAUDE_CODE_SHELL.
//   - Codex reads no variable: on Unix the passwd entry names the shell, on Windows it searches PATH
//     for pwsh, then powershell. SHELL and ComSpec are never read.
//   - Gemini CLI runs `bash` from PATH on Unix; on Windows it uses ComSpec when that names PowerShell,
//     else pwsh.exe from PATH, else powershell.exe.
//
// So PATH is the real lever for Codex and Gemini, and on Windows the shim on it must be a PowerShell.
func agentEnv(kind string, env []string, shim, psShim, goos string) []string {
	dirs := filepath.Dir(shim)
	if goos == "windows" && kind != "claude" {
		dirs = filepath.Dir(psShim) + string(os.PathListSeparator) + dirs
	}
	out := setEnv(env, "PATH", dirs+string(os.PathListSeparator)+envValue(env, "PATH"))
	out = setEnv(out, "INFUSE_LAUNCHED", "1")
	switch {
	case kind == "claude":
		out = setEnv(out, "CLAUDE_CODE_GIT_BASH_PATH", shim)
		out = setEnv(out, "CLAUDE_CODE_SHELL", shim)
		out = setEnv(out, "SHELL", shim)
	case kind == "gemini" && goos == "windows" && comSpecIsPowerShell(envValue(env, "ComSpec")):
		out = setEnv(out, "ComSpec", psShim) // Gemini takes it before PATH when it names PowerShell
	}
	return out
}

func comSpecIsPowerShell(v string) bool {
	low := strings.ToLower(v)
	return strings.HasSuffix(low, "powershell.exe") || strings.HasSuffix(low, "pwsh.exe")
}

func envValue(env []string, key string) string {
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

// setEnv replaces a variable (case-insensitively, as Windows does) instead of appending a duplicate.
func setEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !strings.EqualFold(k, key) {
			out = append(out, e)
		}
	}
	return append(out, key+"="+value)
}

// contextCommand turns the generated per-folder context files on or off.
func contextCommand(root string, args []string) int {
	if len(args) != 1 || args[0] != "on" && args[0] != "off" {
		fmt.Println("usage: infuse context on|off")
		return 1
	}
	if err := projectmap.SetContextFiles(root, args[0] == "on"); err != nil {
		fmt.Println("[infuse] ERROR:", err)
		return 1
	}
	fmt.Printf("[infuse] per-folder context files: %s\n", args[0])
	return 0
}
