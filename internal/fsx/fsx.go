// Package fsx owns the one policy for walking a project: which directories never belong to the
// project's own content, and whether hidden directories are walked. Every mapper, watcher, snapshot,
// oracle and tool uses this package instead of keeping its own list.
package fsx

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// IgnoredDirs are dependency, cache, build-output and infuse's own directories. They are never part of
// a project map, a tree snapshot, a knowledge digest or a file search.
var IgnoredDirs = map[string]bool{
	".cache": true, ".git": true, ".gradle": true, ".idea": true, ".infuse": true, ".next": true,
	".nuxt": true, ".parcel-cache": true, ".turbo": true, ".venv": true, "__pycache__": true,
	"bin": true, "build": true, "coverage": true, "dist": true, "node_modules": true, "obj": true,
	"out": true, "target": true, "vendor": true, "venv": true,
}

// Policy selects the hidden-directory rule on top of IgnoredDirs.
type Policy struct {
	Hidden bool // skip directories whose name starts with a dot
}

var (
	// Project walks the project's own source: ignored directories and hidden directories are out.
	Project = Policy{Hidden: true}
	// Snapshot walks everything that can hold project content: hidden directories are included (for
	// example .github), dependency and build directories are not.
	Snapshot = Policy{}
)

// IsHidden reports whether a directory name is hidden.
func IsHidden(name string) bool { return strings.HasPrefix(name, ".") }

// SkipDir reports whether a directory must not be walked.
func SkipDir(name string, p Policy) bool {
	return IgnoredDirs[name] || (p.Hidden && IsHidden(name))
}

// Walk visits root applying the policy to every directory except root itself. The callback receives
// files and non-skipped directories; it can still return fs.SkipDir for its own depth rules.
func Walk(root string, p Policy, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fn(path, d, err)
		}
		if d.IsDir() && path != root && SkipDir(d.Name(), p) {
			return fs.SkipDir
		}
		return fn(path, d, nil)
	})
}

// IgnoredPath reports whether any component of p below root is ignored or hidden.
func IgnoredPath(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return true
	}
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if IgnoredDirs[seg] || (IsHidden(seg) && seg != "." && seg != "..") {
			return true
		}
	}
	return false
}
