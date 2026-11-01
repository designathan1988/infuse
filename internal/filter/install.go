package filter

// Global, one-time installation. After this, every Claude Code session on this machine runs its
// Bash tool through infuse — desktop app or CLI — with no launcher involved.
//
// The shim is this very executable, copied under the name bash(.exe). When invoked by that name it
// behaves as the interposed shell; invoked as infuse it is the panel/CLI.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func shimPath() string {
	name := "bash"
	if runtime.GOOS == "windows" {
		name = "bash.exe"
	}
	return filepath.Join(core.InfuseHome(), "bin", name)
}

func claudeSettingsPath() string { return filepath.Join(core.HomeDir(), ".claude", "settings.json") }

func readSettings() map[string]any {
	s := map[string]any{}
	_ = core.ReadJSON(claudeSettingsPath(), &s)
	return s
}

func settingsEnv(s map[string]any) map[string]any {
	env, _ := s["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
		s["env"] = env
	}
	return env
}

func envKey() string {
	if runtime.GOOS == "windows" {
		return "CLAUDE_CODE_GIT_BASH_PATH" // Windows: Claude Code always uses Git Bash and finds it here
	}
	return "CLAUDE_CODE_SHELL" // macOS/Linux: explicit shell binary for the Bash tool
}

type InstallResult struct {
	Shim         string `json:"shim"`
	RealBash     string `json:"realBash"`
	SettingsFile string `json:"settingsFile"`
	EnvKey       string `json:"envKey"`
	Error        string `json:"error,omitempty"`
}

// ShimPath is the interposed shell the install role has copied (or will copy) this binary to.
func ShimPath() string { return shimPath() }

// EnsureShim copies this binary under the shell names without touching any settings file: the
// launcher points an external agent's shell at it for one process.
func EnsureShim() error {
	if err := copySelf(shimPath()); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		if err := copySelf(filepath.Join(filepath.Dir(shimPath()), "sh")); err != nil {
			return err
		}
	}
	return nil
}

func Install() InstallResult {
	res := InstallResult{Shim: shimPath(), RealBash: core.FindRealBash(), SettingsFile: claudeSettingsPath(), EnvKey: envKey()}
	if err := copySelf(res.Shim); err != nil {
		res.Error = "cannot install shim: " + err.Error()
		return res
	}
	if runtime.GOOS != "windows" {
		_ = copySelf(filepath.Join(filepath.Dir(res.Shim), "sh"))
	}
	s := readSettings()
	settingsEnv(s)[res.EnvKey] = res.Shim
	if err := core.WriteJSON(res.SettingsFile, s); err != nil {
		res.Error = "cannot write Claude settings: " + err.Error()
	}
	// Example rules the first time, so the panel has something to show and edit.
	if !core.Exists(GlobalRulesFile()) {
		_ = SaveRulesFile(GlobalRulesFile(), ExampleRules)
	}
	return res
}

func Uninstall() InstallResult {
	res := InstallResult{Shim: shimPath(), SettingsFile: claudeSettingsPath(), EnvKey: envKey()}
	s := readSettings()
	if env, ok := s["env"].(map[string]any); ok {
		delete(env, "CLAUDE_CODE_GIT_BASH_PATH")
		delete(env, "CLAUDE_CODE_SHELL")
		if len(env) == 0 {
			delete(s, "env")
		}
	}
	if err := core.WriteJSON(res.SettingsFile, s); err != nil {
		res.Error = err.Error()
	}
	_ = os.RemoveAll(filepath.Dir(res.Shim))
	return res
}

func copySelf(dst string) error {
	src := core.SelfPath()
	if sameFile(src, dst) {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	// Windows cannot overwrite a running executable; write to a temp name and rename.
	tmp := dst + ".new"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	out.Close()
	if core.Exists(dst) {
		_ = os.Rename(dst, dst+".old")
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	_ = os.Remove(dst + ".old")
	return nil
}

func sameFile(a, b string) bool {
	sa, ea := os.Stat(a)
	sb, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(sa, sb)
}

type Status struct {
	Platform      string         `json:"platform"`
	Version       string         `json:"version"`
	Home          string         `json:"home"`
	Shim          string         `json:"shim"`
	ShimInstalled bool           `json:"shimInstalled"`
	RealBash      string         `json:"realBash"`
	SettingsFile  string         `json:"settingsFile"`
	ClaudeEnv     map[string]any `json:"claudeEnv"`
	Pointed       bool           `json:"pointed"`
	EnvKey        string         `json:"envKey"`
}

func GetStatus() Status {
	s := readSettings()
	env, _ := s["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	shim := shimPath()
	return Status{
		Platform: runtime.GOOS, Version: core.Version, Home: core.InfuseHome(),
		Shim: shim, ShimInstalled: core.Exists(shim), RealBash: core.FindRealBash(),
		SettingsFile: claudeSettingsPath(), ClaudeEnv: env, EnvKey: envKey(),
		Pointed: env[envKey()] == shim,
	}
}

func PrintJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}
