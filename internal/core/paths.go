package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const InfuseDirName = ".infuse"

func HomeDir() string {
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
	return filepath.Join(HomeDir(), InfuseDirName)
}

// FindRoot: nearest ancestor holding .infuse/, .git/ or package.json; else cwd.
func FindRoot(cwd string) string {
	if v := os.Getenv("INFUSE_ROOT"); v != "" {
		return v
	}
	dir, _ := filepath.Abs(cwd)
	candidate := ""
	for {
		if Exists(filepath.Join(dir, InfuseDirName)) {
			return dir
		}
		if candidate == "" && (Exists(filepath.Join(dir, ".git")) || Exists(filepath.Join(dir, "package.json")) || Exists(filepath.Join(dir, "go.mod")) || Exists(filepath.Join(dir, "pyproject.toml"))) {
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
		if c != "" && Exists(c) {
			return c
		}
	}
	name := "bash"
	if runtime.GOOS == "windows" {
		name = "bash.exe"
	}
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, name)
		if Exists(p) && !strings.Contains(p, filepath.Join(InfuseDirName, "bin")) {
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
		return filepath.Join(HomeDir(), p[2:])
	}
	return p
}

// ToBashPath converts a Windows path to the form Git Bash accepts in redirections.
func ToBashPath(p string) string {
	if runtime.GOOS != "windows" {
		return p
	}
	if len(p) > 2 && p[1] == ':' {
		return "/" + strings.ToLower(p[:1]) + strings.ReplaceAll(p[2:], `\`, "/")
	}
	return strings.ReplaceAll(p, `\`, "/")
}

func ToPosix(p string) string { return filepath.ToSlash(p) }

// Within reports whether p is root or inside it, comparing whole path components (C:\proj-2 is not in
// C:\proj).
func Within(root, p string) bool {
	if root == "" || p == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// RelTo is the path of abs relative to root, in posix form (empty when they are the same folder).
func RelTo(root, abs string) string {
	r, err := filepath.Rel(root, abs)
	if err != nil {
		return ToPosix(abs)
	}
	if r == "." {
		return ""
	}
	return ToPosix(r)
}
