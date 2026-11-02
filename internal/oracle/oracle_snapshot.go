package oracle

// The snapshot is a real, neutral copy of the working tree, made fresh for each run under a private
// folder. It takes the tree as the disk has it, not as git describes it: a file listed in .gitignore or
// dropped from the index still reaches the checks, and nothing the repository configures (hooks,
// filters) runs, because the copy never goes through checkout. Directories the fixed policy calls build
// output are left out unless git tracks something in them.
//
// Dependency folders are copied too, byte for byte, instead of linked: the chosen trade-off is a slower
// first step for a tree that cannot change under the run. The copy alone would freeze a patched
// node_modules, so each folder's hash is also compared with the anchored baseline (oracle_deps.go) and
// enters the fingerprint a cached verdict is keyed on. A project that wants a clean install declares it
// in "setup".

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"infuse/internal/core"
	"infuse/internal/platform/gitx"
	"infuse/internal/sandbox"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// staleSnapshot is how old a leftover must be before a later run removes it (a crash skips cleanup).
const staleSnapshot = 24 * time.Hour

// snapshotPrefix names snapshots inside the private folder, beside the isolated caches.
const snapshotPrefix = "s-"

// snapshot is one run's frozen tree.
type snapshot struct {
	dir          string
	id           string            // shown with the verdict: the commit it is based on and the content hash
	fingerprint  string            // sha256 of the manifest, dependency folders included
	trees        map[string]string // dependency folder → hash of its content
	outsideLinks int               // links pointing outside the project, not followed
	cleanup      func()
}

func takeSnapshot(workDir, evDir string) (*snapshot, error) {
	parent, err := sandbox.PrivateDir()
	if err != nil {
		return nil, err
	}
	sweepSnapshots(parent)
	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return nil, err
	}
	dir := filepath.Join(parent, snapshotPrefix+hex.EncodeToString(name))
	if err := os.Mkdir(dir, 0o755); err != nil {
		return nil, err
	}
	if err := sandbox.PrepareRoot(dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	c := newCopier(workDir, dir)
	if err := c.run(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	s := &snapshot{dir: dir, fingerprint: c.fingerprint, trees: c.trees, outsideLinks: c.outside, cleanup: func() { _ = os.RemoveAll(dir) }}
	s.id = "sha256:" + s.fingerprint[:12]
	if core.Exists(filepath.Join(workDir, ".git")) {
		neutralRepository(dir)
		s.id = "git:" + short(gitx.Output(workDir, "rev-parse", "--verify", "HEAD")) + "+" + s.fingerprint[:12]
	}
	_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte(c.manifest), 0o644)
	return s, nil
}

// neutralRepository gives the copy of a git project a repository of its own, so tools that expect one
// keep working: fresh configuration, no hooks, and a single commit of what was copied.
func neutralRepository(dir string) {
	if _, err := gitx.Run(dir, "init", "-q"); err != nil {
		return
	}
	var exclude strings.Builder
	for name := range dependencyDirs {
		exclude.WriteString("/" + name + "/\n")
	}
	_ = os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte(exclude.String()), 0o644)
	if _, err := gitx.Run(dir, "add", "-A"); err != nil {
		return
	}
	_, _ = gitx.Run(dir, "-c", "user.name=infuse", "-c", "user.email=oracle@infuse.local", "commit", "-qm", "snapshot", "--no-gpg-sign")
}

// sweepSnapshots removes what an interrupted run left behind.
func sweepSnapshots(parent string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		info, err := e.Info()
		if err == nil && strings.HasPrefix(e.Name(), snapshotPrefix) && time.Since(info.ModTime()) > staleSnapshot {
			_ = os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}

func isWindows() bool { return filepath.Separator == '\\' }

// errSnapshotTooLarge stops a copy that would fill the disk, instead of measuring part of the tree.
var errSnapshotTooLarge = errors.New("the project is too large to snapshot (over 16 GiB outside the build folders)")
