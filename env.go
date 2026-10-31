package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

const infuseDirName = ".infuse"

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "."
}

// InfuseHome is where infuse keeps its global state (shim, rules, projects).
func InfuseHome() string {
	if v := os.Getenv("INFUSE_HOME"); v != "" {
		return v
	}
	return filepath.Join(homeDir(), infuseDirName)
}

// FindRoot: nearest ancestor holding .infuse/, .git/ or package.json; else cwd.
func FindRoot(cwd string) string {
	if v := os.Getenv("INFUSE_ROOT"); v != "" {
		return v
	}
	dir, _ := filepath.Abs(cwd)
	candidate := ""
	for {
		if exists(filepath.Join(dir, infuseDirName)) {
			return dir
		}
		if candidate == "" && (exists(filepath.Join(dir, ".git")) || exists(filepath.Join(dir, "package.json")) || exists(filepath.Join(dir, "go.mod")) || exists(filepath.Join(dir, "pyproject.toml"))) {
			candidate = dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	if candidate != "" {
		return candidate
	}
	abs, _ := filepath.Abs(cwd)
	return abs
}

// FindRealBash: the real bash this machine has, never the shim.
func FindRealBash() string {
	if v := os.Getenv("INFUSE_REAL_BASH"); v != "" {
		return v
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			`C:\Program Files\Git\bin\bash.exe`,
			`C:\Program Files\Git\usr\bin\bash.exe`,
			`C:\Program Files (x86)\Git\bin\bash.exe`,
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "bin", "bash.exe"),
		}
	} else {
		candidates = []string{"/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash", "/opt/homebrew/bin/bash"}
	}
	for _, c := range candidates {
		if c != "" && exists(c) {
			return c
		}
	}
	name := "bash"
	if runtime.GOOS == "windows" {
		name = "bash.exe"
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if exists(p) && !strings.Contains(p, filepath.Join(infuseDirName, "bin")) {
			return p
		}
	}
	if p, err := exec.LookPath("bash"); err == nil {
		return p
	}
	return name
}

var reMsys = regexp.MustCompile(`^/([a-zA-Z])/(.*)$`)

// FromMsysPath converts a Git Bash path to a Windows path, on Windows only. `/c/Users/x` → `C:\Users\x`.
func FromMsysPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if m := reMsys.FindStringSubmatch(p); m != nil {
		return strings.ToUpper(m[1]) + `:\` + strings.ReplaceAll(m[2], "/", `\`)
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(homeDir(), p[2:])
	}
	return p
}

// toBashPath converts a Windows path to the form Git Bash accepts in redirections.
func toBashPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if len(p) > 2 && p[1] == ':' {
		return "/" + strings.ToLower(p[:1]) + strings.ReplaceAll(p[2:], `\`, "/")
	}
	return strings.ReplaceAll(p, `\`, "/")
}

func toPosix(p string) string { return filepath.ToSlash(p) }

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

func readJSON(file string, v any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(file string, v any) error {
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	b, _ := json.MarshalIndent(v, "", "  ")
	return os.WriteFile(file, append(b, '\n'), 0o644)
}

type projectEntry struct {
	Root     string `json:"root"`
	LastSeen int64  `json:"lastSeen"`
	Label    string `json:"label,omitempty"`
	Mode     string `json:"mode,omitempty"` // last permission mode chosen for this folder
}

func projectsFile() string { return filepath.Join(InfuseHome(), "projects.json") }

func loadProjects() []projectEntry {
	var list []projectEntry
	_ = readJSON(projectsFile(), &list)
	if list == nil {
		list = []projectEntry{}
	}
	return list
}

func saveProjects(list []projectEntry) error { return writeJSON(projectsFile(), list) }

// RememberProject records projects the filter has seen, for the panel. A known project keeps the
// position the user gave it; a new one goes first.
func RememberProject(root string) {
	list := loadProjects()
	for i := range list {
		if list[i].Root == root {
			list[i].LastSeen = time.Now().UnixMilli()
			_ = saveProjects(list)
			return
		}
	}
	list = append([]projectEntry{{Root: root, LastSeen: time.Now().UnixMilli()}}, list...)
	if len(list) > 50 {
		list = list[:50]
	}
	_ = saveProjects(list)
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
