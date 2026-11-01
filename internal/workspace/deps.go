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

var dependencyEntrypoints = []string{"node_modules/.bin", ".venv/bin", "venv/bin"}

// DependencyFingerprint hashes the dependency manifests and the files of the installed entrypoint
// directories (the runners a test command executes).
func DependencyFingerprint(work string) string {
	h := sha1.New()
	for _, rel := range dependencyManifests {
		writeDependencyFile(h, work, rel)
	}
	for _, rel := range dependencyEntrypoints {
		base := filepath.Join(work, filepath.FromSlash(rel))
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sort.Strings(names)
		for _, name := range names {
			writeDependencyFile(h, work, rel+"/"+name)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
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
