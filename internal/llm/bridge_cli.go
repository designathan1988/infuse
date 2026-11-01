package llm

// Where the bridged CLIs live on this machine: PATH first, then the usual install folders, then (for
// Codex) the copy bundled with the desktop app, whose versioned MSIX folder is looked up in the package
// registry each time. A CLI that was found is remembered for the process's life; a missing one is
// searched again next time, so installing it needs no restart.

import (
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
)

var (
	bridgeFindMu    sync.Mutex
	bridgeClaudeAlt string
	bridgeCodexAlt  string
)

// FindClaudeCLI locates the Claude Code CLI.
func FindClaudeCLI() string {
	if p := lookCLI("claude", "claude.exe", "claude.cmd", "claude.bat"); p != "" {
		return p
	}
	return cachedCLI("claude", &bridgeClaudeAlt, cliInDir)
}

// FindCodexCLI locates the Codex CLI, including the desktop app's bundled copy.
func FindCodexCLI() string {
	if p := lookCLI("codex", "codex.exe", "codex.cmd", "codex.bat"); p != "" {
		return p
	}
	return cachedCLI("codex", &bridgeCodexAlt, cliInDir)
}

// cliInDir finds the binary — any of its spellings — inside one install folder.
func cliInDir(name, dir string) string {
	for _, stem := range []string{name + ".exe", name + ".cmd", name, name + ".bat"} {
		if p := filepath.Join(dir, stem); fileExists(p) {
			return p
		}
	}
	return ""
}

// cachedCLI memoizes the off-PATH lookup; Codex also tries the copy bundled with its app.
func cachedCLI(name string, cached *string, inDir func(name, dir string) string) string {
	bridgeFindMu.Lock()
	have := *cached
	bridgeFindMu.Unlock()
	if have != "" {
		return have
	}
	var found string
	for _, dir := range fallbackDirs() {
		if found = inDir(name, dir); found != "" {
			break
		}
	}
	if found == "" && name == "codex" {
		found = bundledCodexCLI()
	}
	bridgeFindMu.Lock()
	*cached = found
	bridgeFindMu.Unlock()
	return found
}

// fallbackDirs lists the install folders both CLIs use, beyond PATH.
func fallbackDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, "AppData", "Roaming", "npm"),
		filepath.Join(home, ".npm-global", "bin"),
		"/usr/local/bin",
		"/opt/homebrew/bin",
	}
}

func lookCLI(names ...string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil && !strings.Contains(strings.ToLower(p), core.InfuseDirName) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func bundledCodexCLI() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		"(Get-AppxPackage -Name OpenAI.Codex | Select-Object -First 1).InstallLocation")
	process.NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	loc := strings.TrimSpace(string(out))
	if loc == "" {
		return ""
	}
	p := filepath.Join(loc, "app", "resources", "codex.exe")
	if fileExists(p) {
		return p
	}
	return ""
}

// CodexDefaultModel is the model the local Codex is configured to use (config.toml), for the picker.
func CodexDefaultModel() string {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`(?m)^\s*model\s*=\s*"([^"]+)"`)
	if m := re.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}

// BridgeModels lists what each bridge offers without spending a request: fixed current models for
// Claude, the configured model for Codex.
func BridgeModels(kind string) []string {
	if kind == "codex" {
		m := CodexDefaultModel()
		if m == "" {
			m = "gpt-6.1-sol"
		}
		return []string{m}
	}
	return []string{"claude-sonnet-5-5", "claude-opus-5-5", "claude-haiku-5-5"}
}
