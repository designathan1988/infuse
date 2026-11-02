package oracle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"infuse/internal/platform/process"
	"infuse/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Immutable snapshots with git or a content-addressed copy.

func takeSnapshot(workDir, evDir string) (string, string, func(), error) {
	snap := filepath.Join(os.TempDir(), "infuse-oracle-"+filepath.Base(evDir))
	if core.Exists(filepath.Join(workDir, ".git")) {
		return takeGitSnapshot(workDir, evDir, snap)
	}
	return takeCopySnapshot(workDir, evDir, snap)
}

// takeGitSnapshot commits the whole tree through a temporary index and checks it out in a detached
// worktree under snap.
func takeGitSnapshot(workDir, evDir, snap string) (string, string, func(), error) {
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
		return "", "", nil, errors.New("git add (temporary index) failed")
	}
	// A test hidden behind .gitignore or .git/info/exclude is still the project the user sees. Only
	// source, test and configuration files are forced in: secrets and dependency/build trees must not
	// enter the user's git object store, and no check needs them to prove the project works.
	if files := forceAddCandidates(workDir, git); len(files) > 0 {
		if _, err := git(append([]string{"add", "-f", "--"}, files...)...); err != nil {
			return "", "", nil, errors.New("git add of ignored project files failed")
		}
	}
	tree, err := git("write-tree")
	if err != nil {
		return "", "", nil, errors.New("git write-tree failed: " + tree)
	}
	parent := []string{}
	if head, err := git("rev-parse", "--verify", "HEAD"); err == nil && head != "" {
		parent = []string{"-p", head}
	}
	// the snapshot is infuse's own commit: it must not depend on the machine having a git identity
	args := append([]string{"-c", "user.name=infuse", "-c", "user.email=oracle@infuse.local", "commit-tree", tree}, parent...)
	commit, err := git(append(args, "-m", "infuse oracle snapshot "+filepath.Base(evDir))...)
	if err != nil {
		return "", "", nil, errors.New("git commit-tree failed: " + commit)
	}
	// core.autocrlf=false: the snapshot must hold the same bytes the working tree holds, not a
	// line-ending conversion; otherwise a check measures different content than the user's tree.
	add := exec.Command("git", "-c", "core.autocrlf=false", "worktree", "add", "--detach", snap, commit)
	process.NoConsoleWindow(add)
	add.Dir = workDir
	if out, err := add.CombinedOutput(); err != nil {
		return "", "", nil, errors.New("git worktree add failed: " + strings.TrimSpace(string(out)))
	}
	if diff := snapshotDiff(workDir, snap); len(diff) > 0 { // a smudge filter or a checkout rule changed the content
		_, _ = workspace.GitRun(workDir, "worktree", "remove", "--force", snap)
		if len(diff) > 5 {
			diff = diff[:5]
		}
		return "", "", nil, errors.New("the git snapshot does not match the working tree (filters or checkout rules): " + strings.Join(diff, ", ") + "; refusing the verdict")
	}
	linkDependencies(workDir, snap)
	_ = os.WriteFile(filepath.Join(evDir, "snapshot.txt"), []byte("git commit "+commit+"\ntree "+tree+"\n"), 0o644)
	return snap, "git:" + commit[:12], func() { _, _ = workspace.GitRun(workDir, "worktree", "remove", "--force", snap) }, nil
}

// snapshotSkip reports an ignored file that must stay out of the snapshot: dependencies and build
// output, secrets and oversized files. A test or source file does not match.
func snapshotSkip(work, rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 {
		return true
	}
	skipDir := map[string]bool{"node_modules": true, ".venv": true, "venv": true, "vendor": true, "dist": true,
		"build": true, "out": true, "coverage": true, "target": true, ".next": true, ".cache": true, "__pycache__": true}
	for _, d := range parts[:len(parts)-1] {
		if skipDir[d] {
			return true
		}
	}
	name := strings.ToLower(parts[len(parts)-1])
	exts := map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true,
		".py": true, ".go": true, ".rs": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true,
		".vue": true, ".svelte": true, ".html": true, ".css": true, ".json": true, ".yml": true, ".yaml": true,
		".toml": true, ".md": true}
	if !exts[strings.ToLower(filepath.Ext(name))] || strings.HasPrefix(name, ".env") ||
		name == ".npmrc" || name == ".pypirc" || strings.Contains(name, "credential") ||
		strings.HasSuffix(name, ".pem") || strings.HasSuffix(name, ".key") || strings.HasPrefix(name, "id_rsa") {
		return true
	}
	st, err := os.Stat(filepath.Join(work, filepath.FromSlash(rel)))
	return err != nil || st.Size() > 2<<20
}

// forceAddCandidates lists ignored files that belong to the project's code, tests or configuration.
func forceAddCandidates(workDir string, git func(...string) (string, error)) []string {
	out, err := git("ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return nil
	}
	var files []string
	for _, rel := range strings.Split(out, string(byte(0))) {
		rel = strings.TrimSpace(rel)
		if rel == "" || len(files) >= 5000 || snapshotSkip(workDir, rel) {
			continue
		}
		files = append(files, rel)
	}
	return files
}

// snapshotDiff lists the files whose content differs between the working tree and the snapshot.
func snapshotDiff(workDir, snap string) []string {
	digests := func(base string) map[string]string {
		out := map[string]string{}
		_ = fsx.Walk(base, fsx.Snapshot, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return nil
			}
			rel, _ := filepath.Rel(base, p)
			rel = filepath.ToSlash(rel)
			if rel == ".git" || strings.HasPrefix(rel, ".git/") || strings.HasPrefix(rel, ".infuse/") {
				return nil // a worktree's .git is a file, not a directory the walk skips
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			sum := sha256.Sum256(b)
			out[rel] = hex.EncodeToString(sum[:])
			return nil
		})
		return out
	}
	a, b := digests(workDir), digests(snap)
	var diff []string
	for rel, sum := range a {
		if other, ok := b[rel]; !ok {
			if !snapshotSkip(workDir, rel) { // secrets and oversized ignored files are intentionally absent
				diff = append(diff, "missing "+rel)
			}
		} else if other != sum {
			diff = append(diff, "changed "+rel)
		}
	}
	for rel := range b {
		if _, ok := a[rel]; !ok {
			diff = append(diff, "extra "+rel)
		}
	}
	sort.Strings(diff)
	return diff
}

// takeCopySnapshot copies the tree under snap and returns the sha256 of its manifest.
func takeCopySnapshot(workDir, evDir, snap string) (string, string, func(), error) {
	h := sha256.New()
	var manifest strings.Builder
	err := fsx.Walk(workDir, fsx.Snapshot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := core.RelTo(workDir, p)
		if d.IsDir() {
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
