package sandbox

// What every platform shares: the protected list as paths that exist, and the private temp the
// child's temp and toolchain caches point into.

import (
	"crypto/sha256"
	"encoding/hex"
	"infuse/internal/lang"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// protectedRels turns the protected list into project-relative paths: a literal stays as it is, with
// a trailing /** dropped, and a glob becomes the files it matches now. A glob the kernel cannot read
// has to reach it as paths, or the user's protection would silently cover nothing.
func protectedRels(root string, protected []string) []string {
	var out []string
	var globs []*regexp.Regexp
	for _, p := range protected {
		rel := strings.TrimSpace(strings.TrimSuffix(filepath.ToSlash(p), "/**"))
		switch {
		case rel == "" || filepath.IsAbs(rel):
		case strings.ContainsAny(rel, "*?["):
			globs = append(globs, lang.GlobToRegex(filepath.ToSlash(p)))
		default:
			out = append(out, rel)
		}
	}
	if len(globs) == 0 {
		return out
	}
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		for _, g := range globs {
			if g.MatchString(rel) {
				out = append(out, rel)
				break
			}
		}
		return nil
	})
	return out
}

// sandboxTemp is the project's private writable temp: TMP and the toolchain caches point into it
// (envCacheVars), so builds and package managers work without granting the shared temp. One small
// tree per project, persistent across the commands of that project.
func sandboxTemp(root string) string {
	parent, key := filepath.Join(os.TempDir(), "infuse-sandbox"), strings.ToLower(filepath.Clean(root))
	if isolationKey != "" {
		if dir, err := PrivateDir(); err == nil {
			parent, key = dir, isolationKey
		}
	}
	sum := sha256.Sum256([]byte(key))
	p := filepath.Join(parent, hex.EncodeToString(sum[:8]))
	_ = os.MkdirAll(p, 0o700)
	return p
}

// envCacheVars redirect the child's temp and the known toolchain caches into the private temp.
func envCacheVars(root string) [][2]string {
	t := sandboxTemp(root)
	return [][2]string{
		{"TMP", t}, {"TEMP", t}, {"TMPDIR", t}, {"GOTMPDIR", t},
		{"GOCACHE", filepath.Join(t, "go-build")},
		{"GOPATH", filepath.Join(t, "gopath")},
		{"GOMODCACHE", filepath.Join(t, "gopath", "pkg", "mod")},
		{"GOENV", filepath.Join(t, "goenv")},
		{"GOTELEMETRY", "off"},
		{"npm_config_cache", filepath.Join(t, "npm")},
		{"YARN_CACHE_FOLDER", filepath.Join(t, "yarn")},
		{"PNPM_STORE_DIR", filepath.Join(t, "pnpm-store")},
		{"PIP_CACHE_DIR", filepath.Join(t, "pip")},
		{"CARGO_HOME", filepath.Join(t, "cargo")},
		{"NUGET_PACKAGES", filepath.Join(t, "nuget")},
		{"DOTNET_CLI_HOME", filepath.Join(t, "dotnet")},
		{"GRADLE_USER_HOME", filepath.Join(t, "gradle")},
		{"XDG_CACHE_HOME", filepath.Join(t, "cache")},
		{"PLAYWRIGHT_BROWSERS_PATH", filepath.Join(t, "ms-playwright")},
		{"CYPRESS_CACHE_FOLDER", filepath.Join(t, "cypress")},
	}
}
