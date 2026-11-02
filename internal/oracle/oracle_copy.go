package oracle

// The copy behind a snapshot: which paths go in, and the sha256 manifest of what went. It fails closed:
// a file that cannot be read stops the run, because silently leaving it out is how a test disappears.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"infuse/internal/platform/gitx"
	"infuse/internal/platform/process"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	copyWorkers  = 8
	snapMaxBytes = 16 << 30
)

// copier walks a work tree and, unless dst is empty, copies what the policy admits.
type copier struct {
	work, real, dst string
	depsOnly        bool // hash the dependency folders and nothing else
	trackedDirs     map[string]bool
	hiddenSecrets   map[string]bool // ignored files named like secrets: they stay out of the snapshot
	files           []string
	links           []treeLink
	outside         int

	manifest    string
	fingerprint string
	trees       map[string]string
}

// treeLink is a symbolic link or junction whose target is inside the project.
type treeLink struct {
	rel, target string
	dir         bool
}

func newCopier(work, dst string) *copier {
	real, err := filepath.EvalSymlinks(work)
	if err != nil {
		real = work
	}
	return &copier{work: work, real: real, dst: dst, trackedDirs: trackedDirs(work), hiddenSecrets: ignoredSecrets(work)}
}

// ignoredSecrets lists the git-ignored files that hold credentials: no check needs them to prove the
// project works, and the snapshot is a copy no one else cleans up after a crash. A file that can run
// is never one of them, so naming a test after a secret does not hide it from the checks.
func ignoredSecrets(work string) map[string]bool {
	out := map[string]bool{}
	if !core.Exists(filepath.Join(work, ".git")) {
		return out
	}
	for _, f := range strings.Split(gitx.Output(work, "ls-files", "-z", "--others", "--ignored", "--exclude-standard"), "\x00") {
		if secretName(path.Base(f)) {
			out[f] = true
		}
	}
	return out
}

// secretName recognizes a credential file by its name.
func secretName(name string) bool {
	low := strings.ToLower(name)
	if runnableExts[path.Ext(low)] {
		return false
	}
	return strings.HasPrefix(low, ".env") || low == ".npmrc" || low == ".pypirc" || strings.Contains(low, "credential") ||
		strings.HasSuffix(low, ".pem") || strings.HasSuffix(low, ".key") || strings.HasPrefix(low, "id_rsa")
}

var runnableExts = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".go": true, ".rs": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true, ".sh": true}

// trackedDirs lists the folders holding a file git knows, from the index and from HEAD: a build-like
// folder is copied when the project versions it. Only the walk is widened by this, never narrowed, so
// no change to the index or to .gitignore can remove a file from the snapshot.
func trackedDirs(work string) map[string]bool {
	out := map[string]bool{}
	if !core.Exists(filepath.Join(work, ".git")) {
		return out
	}
	for _, args := range [][]string{{"ls-files", "-z", "--cached"}, {"ls-tree", "-r", "-z", "--name-only", "HEAD"}} {
		for _, f := range strings.Split(gitx.Output(work, args...), "\x00") {
			for d := path.Dir(f); d != "." && d != "/"; d = path.Dir(d) {
				out[d] = true
			}
		}
	}
	return out
}

func (c *copier) run() error {
	if err := c.walk(); err != nil {
		return err
	}
	lines, err := c.copyFiles()
	if err != nil {
		return err
	}
	c.makeLinks()
	for _, l := range c.links {
		lines = append(lines, entryLine{l.rel, "link -> " + l.target})
	}
	c.seal(lines)
	return nil
}

func (c *copier) walk() error {
	return filepath.WalkDir(c.work, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == c.work {
			return nil
		}
		rel := filepath.ToSlash(core.RelTo(c.work, p))
		switch {
		case d.IsDir():
			return c.enterDir(rel, d.Name())
		case c.depsOnly && dependencyOf(rel) == "", d.Name() == ".git":
			return nil
		case c.hiddenSecrets[rel]:
			return nil
		case d.Type().IsRegular():
			c.files = append(c.files, rel)
		default:
			c.noteLink(p, rel)
		}
		return nil
	})
}

// enterDir decides whether a folder goes in. Inside a dependency folder everything counts except the
// caches its tools rebuild; a package's dist or bin there is the installed code itself.
func (c *copier) enterDir(rel, name string) error {
	if name == ".git" || name == core.InfuseDirName {
		return fs.SkipDir
	}
	if dependencyOf(rel) != "" {
		if dependencyCaches[name] && !dependencyDirs[name] {
			return fs.SkipDir
		}
	} else if fsx.IgnoredDirs[name] && !dependencyDirs[name] && !c.trackedDirs[rel] {
		return fs.SkipDir
	}
	if c.dst == "" {
		return nil
	}
	return os.Mkdir(filepath.Join(c.dst, filepath.FromSlash(rel)), 0o755)
}

// noteLink records a link when its target is inside the project. One pointing outside is not followed:
// the copy would hand the checks whatever the link names, a user's private file included.
func (c *copier) noteLink(p, rel string) {
	target, err := filepath.EvalSymlinks(p)
	if err != nil {
		return // dangling, or a device name: nothing to copy
	}
	inner, err := filepath.Rel(c.real, target)
	if err != nil || inner == ".." || strings.HasPrefix(inner, ".."+string(filepath.Separator)) {
		c.outside++
		return
	}
	st, err := os.Stat(target)
	if err != nil || filepath.ToSlash(inner) == rel {
		return
	}
	c.links = append(c.links, treeLink{rel: rel, target: filepath.ToSlash(inner), dir: st.IsDir()})
}

// entryLine is one row of the manifest: a path and what it holds.
type entryLine struct{ rel, sum string }

func (c *copier) copyFiles() ([]entryLine, error) {
	lines := make([]entryLine, len(c.files))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var total atomic.Int64
	var failed atomic.Pointer[error]
	for range copyWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if failed.Load() != nil {
					continue
				}
				sum, n, err := c.copyOne(c.files[i])
				if err == nil && total.Add(n) > snapMaxBytes {
					err = errSnapshotTooLarge
				}
				if err != nil {
					failed.CompareAndSwap(nil, &err)
					continue
				}
				lines[i] = entryLine{c.files[i], sum}
			}
		}()
	}
	for i := range c.files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if e := failed.Load(); e != nil {
		return nil, *e
	}
	return lines, nil
}

// copyOne copies a file while hashing it, so the manifest describes the bytes that were written.
func (c *copier) copyOne(rel string) (string, int64, error) {
	src := filepath.Join(c.work, filepath.FromSlash(rel))
	in, err := os.Open(src)
	if err != nil {
		return "", 0, err
	}
	defer in.Close()
	h := sha256.New()
	var w io.Writer = h
	var out *os.File
	if c.dst != "" {
		info, err := in.Stat()
		if err != nil {
			return "", 0, err
		}
		out, err = os.OpenFile(filepath.Join(c.dst, filepath.FromSlash(rel)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm()|0o200)
		if err != nil {
			return "", 0, err
		}
		defer out.Close()
		w = io.MultiWriter(h, out)
	}
	n, err := io.Copy(w, in)
	if err != nil {
		return "", 0, fmt.Errorf("%s: %w", rel, err)
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// makeLinks recreates the links inside the copy, pointing at the copy. A platform that refuses links
// gets a junction for a folder and the file's content for a file.
func (c *copier) makeLinks() {
	if c.dst == "" {
		return
	}
	for _, l := range c.links {
		dst := filepath.Join(c.dst, filepath.FromSlash(l.rel))
		target := filepath.Join(c.dst, filepath.FromSlash(l.target))
		if os.Symlink(target, dst) == nil || !isWindows() {
			continue
		}
		if l.dir {
			cmd := exec.Command("cmd", "/c", "mklink", "/J", dst, target) // junction: no privilege needed
			process.NoConsoleWindow(cmd)
			_ = cmd.Run()
		} else if b, err := os.ReadFile(target); err == nil {
			_ = os.WriteFile(dst, b, 0o644)
		}
	}
}

// seal builds the manifest: a line per project file, and one per dependency folder carrying the hash of
// its content, because thousands of lines per run would only bloat the evidence.
func (c *copier) seal(lines []entryLine) {
	folders := map[string]*depTree{}
	var m strings.Builder
	for _, e := range lines {
		dep := dependencyOf(e.rel)
		if dep == "" {
			m.WriteString(e.sum + "  " + e.rel + "\n")
			continue
		}
		t := folders[dep]
		if t == nil {
			t = &depTree{h: sha256.New()}
			folders[dep] = t
		}
		fmt.Fprintf(t.h, "%s  %s\n", e.sum, e.rel)
		t.n++
	}
	c.trees = map[string]string{}
	for _, name := range sortedKeys(folders) {
		t := folders[name]
		c.trees[name] = hex.EncodeToString(t.h.Sum(nil))
		fmt.Fprintf(&m, "%s  %s/ (%d files)\n", c.trees[name], name, t.n)
	}
	c.manifest = m.String()
	sum := sha256.Sum256([]byte(c.manifest))
	c.fingerprint = hex.EncodeToString(sum[:])
}
