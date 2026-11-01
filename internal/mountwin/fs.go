//go:build windows && !cgo

// Package mountwin exposes a project through a WinFsp drive. Every write passes the same pre-write
// policy as the filter: .infuse and the protected paths are refused, and code outside the chosen base
// is refused. The filesystem runs in the infuse process, so the policy is the write perimeter for
// tools that write through the mount.

package mountwin

import (
	"errors"
	"infuse/internal/base"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/winfsp/cgofuse/fuse"
)

const maxHandles = 4096

type FileSystem struct {
	fuse.FileSystemBase
	root      string
	base      *base.Base
	protected []*regexp.Regexp
	mu        sync.Mutex
	handles   map[uint64]*openFile
	next      uint64
}

type openFile struct {
	file  *os.File
	rel   string
	bytes int
	write bool
}

// New builds the passthrough filesystem and caches the base's policy for the life of the mount.
func New(root string, protected []string) (*FileSystem, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	if st, err := os.Stat(abs); err != nil || !st.IsDir() {
		return nil, errors.New("the mount root is not a directory")
	}
	fs := &FileSystem{root: abs, base: base.Load(abs), handles: map[uint64]*openFile{}}
	for _, pattern := range protected {
		pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
		if pattern != "" {
			fs.protected = append(fs.protected, lang.GlobToRegex(pattern))
		}
	}
	return fs, nil
}

// realPath maps a FUSE path to the work root and refuses traversal outside it.
func (fs *FileSystem) realPath(p string) (abs, rel string, errc int) {
	p = strings.ReplaceAll(p, "\\", "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	clean := path.Clean(p)
	if clean == "/" {
		return fs.root, ".", 0
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", "", -fuse.EACCES
	}
	rel = strings.TrimPrefix(clean, "/")
	abs = filepath.Join(fs.root, filepath.FromSlash(rel))
	if out, err := filepath.Rel(fs.root, abs); err != nil || out == ".." || strings.HasPrefix(out, ".."+string(filepath.Separator)) {
		return "", "", -fuse.EACCES
	}
	return abs, rel, 0
}

// writePath resolves symlinks and junctions and refuses any target that lands outside the root or in
// a protected path. Without it a link created inside the project could make the mount process write
// outside the root that the OS sandbox protects.
func (fs *FileSystem) writePath(p string, isDir bool) (string, int) {
	abs, _, errc := fs.realPath(p)
	if errc != 0 {
		return "", errc
	}
	resolved, err := evalWithinRoot(abs)
	if err != nil || resolved == fs.root {
		return "", -fuse.EACCES
	}
	rel, err := filepath.Rel(fs.root, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", -fuse.EACCES
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		rel = ""
	}
	if errc := fs.allowWrite(rel, isDir); errc != 0 {
		return "", errc
	}
	return resolved, 0
}

// evalWithinRoot resolves the deepest existing ancestor of a path; for a new file this resolves its
// directory, so a junction in the parent chain cannot hide an outside target.
func evalWithinRoot(abs string) (string, error) {
	var missing []string
	current := abs
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("no existing ancestor")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func (fs *FileSystem) isProtected(rel string) bool {
	if rel == ".infuse" || strings.HasPrefix(rel, ".infuse/") {
		return true
	}
	for _, re := range fs.protected {
		if re.MatchString(rel) {
			return true
		}
	}
	return false
}

// allowWrite is the pre-write policy; it returns 0 or a negated FUSE error.
func (fs *FileSystem) allowWrite(rel string, isDir bool) int {
	if fs.isProtected(rel) {
		return -fuse.EPERM
	}
	if !isDir && strings.Contains(rel, "/") && isCodeFile(rel) && fs.base != nil && !fs.base.Declares(rel) {
		return -fuse.EPERM
	}
	return 0
}

var codeExtensions = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".kts": true, ".cs": true, ".rb": true,
	".php": true, ".c": true, ".h": true, ".cc": true, ".cpp": true, ".hpp": true, ".swift": true,
	".lua": true, ".sh": true, ".bash": true, ".ps1": true, ".vue": true, ".svelte": true,
}

func isCodeFile(rel string) bool {
	return codeExtensions[strings.ToLower(filepath.Ext(rel))]
}

func (fs *FileSystem) addHandle(f *os.File, rel string, write bool) (uint64, int) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.handles) >= maxHandles {
		return 0, -fuse.EMFILE
	}
	fs.next++
	fh := fs.next
	fs.handles[fh] = &openFile{file: f, rel: rel, write: write}
	return fh, 0
}

func (fs *FileSystem) handle(fh uint64) *openFile {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.handles[fh]
}

func (fs *FileSystem) dropHandle(fh uint64) *openFile {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	f := fs.handles[fh]
	delete(fs.handles, fh)
	return f
}

func (fs *FileSystem) Destroy() { fs.closeHandles() }

// closeHandles runs on unmount, so a killed mount does not leak open files.
func (fs *FileSystem) closeHandles() {
	fs.mu.Lock()
	handles := fs.handles
	fs.handles = map[uint64]*openFile{}
	fs.mu.Unlock()
	for _, h := range handles {
		_ = h.file.Close()
		if h.write {
			fs.journalWrite(h.rel, h.bytes)
		}
	}
}

func (fs *FileSystem) journalWrite(rel string, bytes int) {
	if rel == "" || rel == "." {
		return
	}
	journal.Journal(fs.root, journal.JournalEntry{Kind: "write", Path: rel, Bytes: bytes})
}

func (fs *FileSystem) journalKind(kind, rel, detail string) {
	if rel == "" || rel == "." {
		return
	}
	journal.Journal(fs.root, journal.JournalEntry{Kind: kind, Path: rel, Detail: detail})
}

func errno(err error) int {
	switch {
	case err == nil:
		return 0
	case os.IsNotExist(err):
		return -fuse.ENOENT
	case os.IsPermission(err):
		return -fuse.EACCES
	case os.IsExist(err):
		return -fuse.EEXIST
	case errors.Is(err, syscall.ENOTEMPTY):
		return -fuse.ENOTEMPTY
	default:
		return -fuse.EIO
	}
}

func statOf(info os.FileInfo) *fuse.Stat_t {
	st := &fuse.Stat_t{Nlink: 1, Size: info.Size()}
	mode := uint32(info.Mode().Perm())
	switch {
	case info.IsDir():
		mode |= fuse.S_IFDIR
	case info.Mode()&os.ModeSymlink != 0:
		mode |= fuse.S_IFLNK
	default:
		mode |= fuse.S_IFREG
	}
	st.Mode = mode
	ts := fuse.NewTimespec(info.ModTime())
	st.Atim, st.Mtim, st.Ctim = ts, ts, ts
	return st
}

func osFlags(flags int) int {
	out := os.O_RDONLY
	switch flags & fuse.O_ACCMODE {
	case fuse.O_WRONLY:
		out = os.O_WRONLY
	case fuse.O_RDWR:
		out = os.O_RDWR
	}
	if flags&fuse.O_APPEND != 0 {
		out |= os.O_APPEND
	}
	if flags&fuse.O_CREAT != 0 {
		out |= os.O_CREATE
	}
	if flags&fuse.O_EXCL != 0 {
		out |= os.O_EXCL
	}
	if flags&fuse.O_TRUNC != 0 {
		out |= os.O_TRUNC
	}
	return out
}
