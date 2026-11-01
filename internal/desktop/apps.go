package desktop

// Other agent apps on this machine, detected and launched from the window. MSIX desktop apps (the modern
// ChatGPT and Claude installs) have no discoverable .exe: they appear in the Start-app list
// (Get-StartApps) and launch through shell:AppsFolder. CLIs are found on PATH and in npm's global bin and
// open in a terminal at the project folder. Nothing is hard-coded: AppIDs come from the enumeration.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

type App struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Kind  string `json:"kind"`           // msix | desktop | cli
	Path  string `json:"path,omitempty"` // exe path (desktop/cli) or shell:AppsFolder id (msix)
	AppID string `json:"appId,omitempty"`
	Found bool   `json:"found"`
	Note  string `json:"note,omitempty"`
}

// CliPackages: the official npm packages, for "Instalar" when a CLI is missing.
var CliPackages = map[string]string{"claude-cli": "@anthropic-ai/claude-code", "codex-cli": "@openai/codex", "gemini-cli": "@google/gemini-cli"}

type startApp struct {
	Name  string
	AppID string
}

var (
	appsCache   []App
	appsCacheAt time.Time
	appsMu      sync.Mutex
)

// DetectApps lists the agent apps this machine has (cached briefly: Get-StartApps is slow).
func DetectApps() []App {
	appsMu.Lock()
	defer appsMu.Unlock()
	if appsCache != nil && time.Since(appsCacheAt) < 30*time.Second {
		return appsCache
	}
	appsCache = detectApps(startApps())
	appsCacheAt = time.Now()
	return appsCache
}

func detectApps(start []startApp) []App {
	home, _ := os.UserHomeDir()
	apps := []App{
		{ID: "claude-desktop", Name: "Claude (desktop)", Kind: "msix"},
		{ID: "chatgpt-desktop", Name: "ChatGPT (desktop)", Kind: "msix"},
		{ID: "claude-cli", Name: "Claude Code (CLI)", Kind: "cli", Note: "opens in a terminal in the project folder"},
		{ID: "codex-cli", Name: "Codex CLI", Kind: "cli", Note: "opens in a terminal in the project folder"},
		{ID: "gemini-cli", Name: "Gemini CLI", Kind: "cli", Note: "opens in a terminal in the project folder"},
	}
	for i := range apps {
		a := &apps[i]
		switch a.ID {
		case "claude-desktop":
			if sa := findStartApp(start, `^Claude$`, "_crx_"); sa != nil {
				a.AppID, a.Found, a.Path = sa.AppID, true, "shell:AppsFolder\\"+sa.AppID
				continue
			}
			for _, p := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "AnthropicClaude", "claude.exe"), filepath.Join(os.Getenv("ProgramFiles"), "Claude", "Claude.exe"), "/Applications/Claude.app"} {
				if appFileExists(p) {
					a.Kind, a.Path, a.Found = "desktop", p, true
					break
				}
			}
		case "chatgpt-desktop":
			if sa := findStartApp(start, `^ChatGPT$`, "_crx_"); sa != nil {
				a.AppID, a.Found, a.Path = sa.AppID, true, "shell:AppsFolder\\"+sa.AppID
				continue
			}
			for _, p := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "OpenAI", "ChatGPT", "ChatGPT.exe"), "/Applications/ChatGPT.app"} {
				if appFileExists(p) {
					a.Kind, a.Path, a.Found = "desktop", p, true
					break
				}
			}
		case "claude-cli":
			a.Path = findCLI("claude", home)
		case "codex-cli":
			a.Path = findCLI("codex", home)
			if a.Path == "" {
				a.Path = bundledCodex()
				if a.Path != "" {
					a.Note = "o CLI que vem com o app Codex"
				}
			}
		case "gemini-cli":
			a.Path = findCLI("gemini", home)
		}
		if a.Kind == "cli" && a.Path != "" {
			a.Found = true
		}
	}
	return apps
}

// quoteAll quotes each command word for cmd.exe's /K line.
func quoteAll(words []string) []string {
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = `"` + w + `"`
	}
	return out
}

func appFileExists(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(p)
	return err == nil && (!st.IsDir() || strings.HasSuffix(p, ".app"))
}

// findStartApp matches a Start-app by name, skipping AppIDs containing exclude (Chrome PWAs: "_crx_").
func findStartApp(list []startApp, nameRe, exclude string) *startApp {
	re := regexp.MustCompile(nameRe)
	for i := range list {
		sa := &list[i]
		if exclude != "" && strings.Contains(sa.AppID, exclude) {
			continue
		}
		if re.MatchString(strings.TrimSpace(sa.Name)) {
			return sa
		}
	}
	return nil
}

func findCLI(name, home string) string {
	cands := []string{name}
	if runtime.GOOS == "windows" {
		cands = []string{name + ".exe", name + ".cmd", name + ".bat"}
	}
	for _, c := range cands {
		if p, err := exec.LookPath(c); err == nil && !strings.Contains(strings.ToLower(p), core.InfuseDirName) {
			return p
		}
		for _, dir := range []string{filepath.Join(home, "AppData", "Roaming", "npm"), filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".npm-global"), filepath.Join(home, ".local", "bin"), "/usr/local/bin", "/opt/homebrew/bin"} {
			if p := filepath.Join(dir, c); appFileExists(p) {
				return p
			}
		}
	}
	return ""
}

// CLIPath is the executable of one agent CLI ("claude", "codex", "gemini"), as detected on this machine.
func CLIPath(kind string) string {
	for _, a := range DetectApps() {
		if a.ID == kind+"-cli" && a.Kind == "cli" && a.Found {
			return a.Path
		}
	}
	return ""
}

// LaunchApp starts an app: MSIX via shell:AppsFolder, desktop apps directly, CLIs in a new terminal at
// workdir — always through infuse's launcher, so the agent's shell passes through the filter.
func LaunchApp(id, workdir string) (string, error) {
	var app App
	for _, a := range DetectApps() {
		if a.ID == id {
			app = a
		}
	}
	if app.ID == "" {
		return "", fmt.Errorf("app desconhecido: %s", id)
	}
	if !app.Found {
		return "", fmt.Errorf("%s was not found on this machine", app.Name)
	}
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	var cmd *exec.Cmd
	switch app.Kind {
	case "msix":
		cmd = exec.Command("explorer.exe", "shell:AppsFolder\\"+app.AppID)
	case "desktop":
		if runtime.GOOS == "darwin" {
			cmd = exec.Command("open", "-a", app.Path)
		} else {
			cmd = exec.Command(app.Path)
		}
		cmd.Dir = workdir
	case "cli":
		// Never start the CLI onto the project raw: infuse's launcher installs the shim and points the
		// agent's shell at it for that process.
		kind := strings.TrimSuffix(app.ID, "-cli")
		launcher := []string{core.SelfPath(), kind}
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/C", "start", "", "/D", workdir, "cmd", "/K", strings.Join(quoteAll(launcher), " "))
		case "darwin":
			cmd = exec.Command("osascript", "-e", `tell application "Terminal" to do script "cd `+asEscape(workdir)+` && `+asEscape(strings.Join(launcher, " "))+`"`, "-e", `tell application "Terminal" to activate`)
		default:
			cmd = exec.Command("x-terminal-emulator", "-e", strings.Join(launcher, " "))
			cmd.Dir = workdir
		}
	}
	process.NoConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { _ = cmd.Wait() }()
	if app.AppID != "" {
		return app.AppID, nil
	}
	return app.Path, nil
}
