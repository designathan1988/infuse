package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"infuse/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Immutable snapshots with git or a content-addressed copy.

func takeSnapshot(workDir, evDir string) (string, string, func(), error) {
	snap := filepath.Join(os.TempDir(), "infuse-oracle-"+filepath.Base(evDir))
	if core.Exists(filepath.Join(workDir, ".git")) {
		idx := filepath.Join(os.TempDir(), "infuse-index-"+filepath.Base(evDir))
		defer os.Remove(idx)
		git := func(args ...string) (string, error) {
			cmd := exec.Command("git", args...)
			process.NoConsoleWindow(cmd)
			cmd.Dir = workDir
			cmd.Env = append(os.Environ(), "GIT_INDEX_FILE="+idx)
			out, err := cmd.CombinedOutput()
			return strings.TrimSpace(string(out)), err
		}
		if _, err := git("add", "-A", "--", ".", ":(exclude).infuse"); err != nil {
			return "", "", nil, errors.New("git add (índice temporário) falhou")
		}
		tree, err := git("write-tree")
		if err != nil {
			return "", "", nil, errors.New("git write-tree falhou: " + tree)
		}
		parent := []string{}
		if head, err := git("rev-parse", "--verify", "HEAD"); err == nil && head != "" {
			parent = []string{"-p", head}
		}
		commit, err := git(append(append([]string{"commit-tree", tree}, parent...), "-m", "infuse oracle snapshot "+filepath.Base(evDir))...)
		if err != nil {
			return "", "", nil, errors.New("git commit-tree falhou: " + commit)
		}
		if out, err := workspace.GitRun(workDir, "worktree", "add", "--detach", snap, commit); err != nil {
			return "", "", nil, errors.New("git worktree add falhou: " + strings.TrimSpace(out))
		}
		linkDependencies(workDir, snap)
		_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte("git commit "+commit+"\ntree "+tree+"\n"), 0o644)
		return snap, "git:" + commit[:12], func() { _, _ = workspace.GitRun(workDir, "worktree", "remove", "--force", snap) }, nil
	}
	h := sha256.New()
	var manifest strings.Builder
	err := filepath.WalkDir(workDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := core.RelTo(workDir, p)
		if d.IsDir() {
			if rel != "" && (projectmap.IgnoredDirs[d.Name()] || d.Name() == core.InfuseDirName) {
				return filepath.SkipDir
			}
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(b)
		line := hex.EncodeToString(sum[:]) + "  " + filepath.ToSlash(rel) + "\n"
		manifest.WriteString(line)
		h.Write([]byte(line))
		dst := filepath.Join(snap, rel)
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		return "", "", nil, err
	}
	linkDependencies(workDir, snap)
	_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte(manifest.String()), 0o644)
	id := hex.EncodeToString(h.Sum(nil))
	return snap, "sha256:" + id[:12], func() { _ = os.RemoveAll(snap) }, nil
}

// linkDependencies points installed dependency folders (node_modules, .venv, vendor) of the project
// into the snapshot, read through a link: checks run against the code snapshot with the dependencies
// already installed, without copying gigabytes.
func linkDependencies(from, snap string) {
	for _, d := range []string{"node_modules", ".venv", "venv"} {
		src := filepath.Join(from, d)
		if st, err := os.Stat(src); err == nil && st.IsDir() && !core.Exists(filepath.Join(snap, d)) {
			if err := os.Symlink(src, filepath.Join(snap, d)); err != nil && isWindows() {
				cmd := exec.Command("cmd", "/c", "mklink", "/J", filepath.Join(snap, d), src) // junction: no privilege needed
				process.NoConsoleWindow(cmd)
				_ = cmd.Run()
			}
		}
	}
}

func isWindows() bool { return filepath.Separator == '\\' }

// ListVerdicts returns the most recent runs of a project, newest first.
