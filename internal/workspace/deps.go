package workspace

// Dependency fingerprint: the manifests and the installed entrypoints a check could execute. The
// executor runs against a snapshot that junctions the live dependency directories in; if a lockfile or
// an installed runner changes while the checks run, the verdict was produced against something else and
// is refused.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

var dependencyManifests = []string{
	"package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock",
	"go.mod", "go.sum", "poetry.lock", "requirements.txt", "pyproject.toml",
	"Pipfile.lock", "Cargo.lock",
}

var dependencyDirs = []string{"node_modules", ".venv", "venv", "vendor"}

// DependencyFingerprint hashes the dependency manifests and every installed file a check could
// execute or import. A patch to a runner or to a library it imports changes the fingerprint even when
// the lockfile and the .bin entries stay identical.
func DependencyFingerprint(work string) string {
	h := sha1.New()
	for _, rel := range dependencyManifests {
		writeDependencyFile(h, work, rel)
	}
	for _, dir := range dependencyDirs {
		hashDependencyTree(h, work, dir)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func hashDependencyTree(h io.Writer, work, dir string) {
	base := filepath.Join(work, filepath.FromSlash(dir))
	if st, err := os.Stat(base); err != nil || !st.IsDir() {
		return
	}
	var paths []string
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != base && (d.Name() == ".cache" || d.Name() == "__pycache__" || d.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() || d.Type()&os.ModeSymlink != 0 {
			paths = append(paths, p)
		}
		return nil
	})
	sort.Strings(paths)
	for _, p := range paths {
		rel, _ := filepath.Rel(work, p)
		writeDependencyFile(h, work, filepath.ToSlash(rel))
	}
}

func writeDependencyFile(h io.Writer, work, rel string) {
	p := filepath.Join(work, filepath.FromSlash(rel))
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return
	}
	if b, err := os.ReadFile(p); err == nil {
		_, _ = io.WriteString(h, rel)
		_, _ = h.Write([]byte{0})
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
		return
	}
	if st, err := os.Lstat(p); err == nil { // an unreadable entry still has an identity
		_, _ = io.WriteString(h, fmt.Sprintf("%s:%d:%d", rel, st.Size(), st.ModTime().UnixNano()))
	}
}
