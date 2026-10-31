package workspace

// Turn snapshots for folders without git: at the start of every turn infuse records the whole tree
// (path → content hash, contents kept once in a content-addressed store, like git objects). Changes are
// then measured against what the tree really was, whatever made them: the agent's Write and Edit, a
// shell command (rm, mv, sed -i, a generator), or the user. A file removed by any means shows as
// deleted with all its lines; a file renamed shows as one removed and one added.

import (
	"crypto/sha1"
	"encoding/hex"
	"infuse/internal/core"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	snapMaxFiles = 20000
	snapMaxSize  = 2 << 20
)

// dirs that hold dependencies, caches or build output, never the project's own work
var snapSkipDirs = map[string]bool{".git": true, ".infuse": true, "node_modules": true, "__pycache__": true, ".venv": true, "venv": true, ".cache": true, ".next": true, ".nuxt": true, ".turbo": true, ".parcel-cache": true, ".gradle": true, ".idea": true, "target": true}

type treeManifest struct {
	Work  string            `json:"work"`
	At    int64             `json:"at"`
	Turn  int               `json:"turn"`
	Files map[string]string `json:"files"` // slash path → sha1 of the content
}

func treeDir(root string) string { return filepath.Join(root, core.InfuseDirName, "tree") }

func blobPath(root, hash string) string {
	return filepath.Join(treeDir(root), "blobs", hash[:2], hash)
}

func hashBytes(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

var (
	hashCacheMu sync.Mutex
	hashCache   = map[string]hashEntry{} // abs path → hash of the content at that mtime and size
)

type hashEntry struct {
	mod  int64
	size int64
	hash string
}

// walkTree hashes every project file of work (skipping dependency and build folders). With store, the
// contents go to root's blob store.
func walkTree(root, work string, store bool) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(work, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != work && snapSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= snapMaxFiles || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > snapMaxSize {
			return nil
		}
		rel, _ := filepath.Rel(work, p)
		rel = filepath.ToSlash(rel)
		hashCacheMu.Lock()
		e, ok := hashCache[p]
		hashCacheMu.Unlock()
		settled := time.Since(info.ModTime()) > 2*time.Second // a just-written file may change again within the same mtime
		if ok && settled && e.mod == info.ModTime().UnixNano() && e.size == info.Size() && (!store || core.Exists(blobPath(root, e.hash))) {
			out[rel] = e.hash
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		h := hashBytes(b)
		if store {
			bp := blobPath(root, h)
			if !core.Exists(bp) {
				_ = os.MkdirAll(filepath.Dir(bp), 0o755)
				_ = os.WriteFile(bp, b, 0o644)
			}
		}
		hashCacheMu.Lock()
		hashCache[p] = hashEntry{info.ModTime().UnixNano(), info.Size(), h}
		hashCacheMu.Unlock()
		out[rel] = h
		return nil
	})
	return out
}

// snapshotTurn records the tree at the start of a session's turn.
func SnapshotTurn(root, work, session string, turn int) {
	m := treeManifest{Work: filepath.Clean(work), At: time.Now().UnixMilli(), Turn: turn, Files: walkTree(root, work, true)}
	_ = core.WriteJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), m)
}

// manifests lists the recorded snapshots of a work folder, oldest first (only one session's with session).
func Manifests(root, work, session string) []treeManifest {
	var out []treeManifest
	dirs, _ := os.ReadDir(treeDir(root))
	for _, d := range dirs {
		if !d.IsDir() || d.Name() == "blobs" || session != "" && d.Name() != session {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(treeDir(root), d.Name()))
		for _, f := range files {
			var m treeManifest
			if strings.HasSuffix(f.Name(), ".json") && core.ReadJSON(filepath.Join(treeDir(root), d.Name(), f.Name()), &m) == nil && strings.EqualFold(m.Work, filepath.Clean(work)) {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

func ReadBlob(root, hash string) (string, bool) {
	b, err := os.ReadFile(blobPath(root, hash))
	return string(b), err == nil
}

// SnapSkipDirs are the folders a tree snapshot never walks.
var SnapSkipDirs = snapSkipDirs
