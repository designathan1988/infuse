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
	"infuse/internal/fsx"
	"infuse/internal/journal"
	"io"
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
	// Large enough that the audit does not go blind on a real project: every file up to these limits is
	// compared by content. Above them the file is still hashed, but its old content is not stored.
	snapMaxFiles = 100000
	snapMaxSize  = 16 << 20
)

type treeManifest struct {
	Work         string            `json:"work"`
	At           int64             `json:"at"`
	Turn         int               `json:"turn"`
	Files        map[string]string `json:"files"`        // slash path → sha1 of the content
	Protected    map[string]string `json:"protected"`    // .infuse/oracle and .infuse/evidence → sha1
	Dependencies string            `json:"dependencies"` // lockfiles + installed entrypoints at the turn start
}

func treeDir(root string) string { return filepath.Join(root, core.InfuseDirName, "tree") }

func blobPath(root, hash string) string {
	return filepath.Join(treeDir(root), "blobs", hash[:2], hash)
}

func hashBytes(b []byte) string {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])
}

// hashFile hashes a file without loading it whole (a generated file can be very large).
func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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

// walkTree hashes every project file of work (skipping dependency and build folders) by content. With
// store, the contents go to root's blob store. A size and mtime cache cannot serve the audit: a same-size
// edit with its timestamp restored would be invisible, so only the changes pane, which decides nothing,
// reads through it (walkTreeCached).
func walkTree(root, work string, store bool) map[string]string {
	return walkTreeWith(root, work, store, false)
}

// walkTreeCached is walkTree for display: it reuses the hash of a file whose size and mtime are unchanged.
func walkTreeCached(root, work string) map[string]string {
	return walkTreeWith(root, work, false, true)
}

func walkTreeWith(root, work string, store, useCache bool) map[string]string {
	out := map[string]string{}
	_ = fsx.Walk(work, fsx.Snapshot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if len(out) >= snapMaxFiles || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(work, p)
		rel = filepath.ToSlash(rel)
		hashCacheMu.Lock()
		e, ok := hashCache[p]
		hashCacheMu.Unlock()
		settled := time.Since(info.ModTime()) > 2*time.Second // a just-written file may change again within the same mtime
		if useCache && ok && settled && e.mod == info.ModTime().UnixNano() && e.size == info.Size() && (!store || core.Exists(blobPath(root, e.hash))) {
			out[rel] = e.hash
			return nil
		}
		if info.Size() > snapMaxSize { // hashed in a stream and never stored: a very large file is still a change
			if h, err := hashFile(p); err == nil {
				hashCacheMu.Lock()
				hashCache[p] = hashEntry{info.ModTime().UnixNano(), info.Size(), h}
				hashCacheMu.Unlock()
				out[rel] = h
			}
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

// SnapshotTurn records the tree at the start of a session's turn. The error is returned so the
// session can fail closed: without the initial tree the turn cannot be audited.
func SnapshotTurn(root, work, session string, turn int) error {
	protected := protectedHashes(root)
	m := treeManifest{
		Work: filepath.Clean(work), At: time.Now().UnixMilli(), Turn: turn,
		Files: walkTree(root, work, true), Protected: protected,
		Dependencies: DependencyFingerprint(work),
	}
	storeProtectedBlobs(root, protected)
	if err := core.WriteJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), m); err != nil {
		journal.Journal(root, journal.JournalEntry{Kind: "gate", Detail: "turn snapshot failed: " + err.Error()})
		return err
	}
	PruneTree(root) // keep the newest turns of each session; drop content no kept manifest refers to
	return nil
}

// protectedHashes hashes what the agent must never write: the oracle's criteria and the executor's
// evidence. The shell guard reads the command line; a script that writes the file directly is caught
// by comparing these hashes at the end of the turn. Always by content and with no size or count limit:
// a limit is a place to hide a change.
// storeProtectedBlobs keeps the content of the oracle and evidence so a deletion can be undone, not
// only reported. A protected file too large for the cache is still hashed and audited.
func storeProtectedBlobs(root string, protected map[string]string) {
	for rel, hash := range protected {
		bp := blobPath(root, hash)
		if core.Exists(bp) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		_ = os.MkdirAll(filepath.Dir(bp), 0o755)
		_ = os.WriteFile(bp, b, 0o644)
	}
}

// RestoreProtected puts back the listed oracle and evidence files as the turn recorded them.
func RestoreProtected(root, session string, turn int, paths []string) {
	var m treeManifest
	if core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), &m) != nil {
		return
	}
	for _, rel := range paths {
		b, ok := ReadBlob(root, m.Protected[rel])
		if !ok {
			continue
		}
		dst := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, []byte(b), 0o644)
	}
}

func protectedHashes(root string) map[string]string {
	out := map[string]string{}
	for _, dir := range []string{"oracle", "evidence"} {
		base := filepath.Join(root, core.InfuseDirName, dir)
		_ = filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(root, p)
			h, err := hashFile(p)
			if err != nil {
				h = "unreadable" // a file that cannot be read is not a file that did not change
			}
			out[filepath.ToSlash(rel)] = h
			return nil
		})
	}
	return out
}

// TurnProtectedChanges lists oracle/evidence files that changed or vanished since the turn started: a
// deleted criterion is the cheapest way to disarm the oracle. The executor's own writes are accepted by
// refreshing the manifest after each run (RefreshTurnProtected).
func TurnProtectedChanges(root, session string, turn int) []string {
	var m treeManifest
	if core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), &m) != nil {
		return nil
	}
	now := protectedHashes(root)
	var out []string
	for rel, h := range now {
		if old, had := m.Protected[rel]; !had || old != h {
			out = append(out, rel)
		}
	}
	for rel := range m.Protected {
		if _, still := now[rel]; !still {
			out = append(out, rel) // the file no longer exists: a deletion is a protected change too
		}
	}
	sort.Strings(out)
	return out
}

// RefreshTurnProtected moves into the turn's baseline only the protected paths accept approves: what
// infuse itself just wrote. Anything else keeps its old hash, so a criteria file changed by the agent
// stays reported however many times the executor runs.
func RefreshTurnProtected(root, session string, turn int, accept func(rel string) bool) {
	p := filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json")
	var m treeManifest
	if core.ReadJSON(p, &m) != nil {
		return
	}
	if m.Protected == nil {
		m.Protected = map[string]string{}
	}
	for rel, h := range protectedHashes(root) {
		if accept(rel) {
			m.Protected[rel] = h
		}
	}
	storeProtectedBlobs(root, m.Protected)
	_ = core.WriteJSON(p, m)
}

// TreeChange is one path the tree differs by since the turn started: changed, created or removed,
// whatever made the change (a write tool, a shell command, a generator, another process).
type TreeChange struct {
	Path    string
	Before  string // hash of the content at the start of the turn ("" when the file did not exist)
	New     bool
	Deleted bool
}

// TurnChanges compares the current tree with the snapshot taken when the turn started.
func TurnChanges(root, work, session string, turn int) []TreeChange {
	var m treeManifest
	if core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), &m) != nil {
		return nil
	}
	if !strings.EqualFold(filepath.Clean(m.Work), filepath.Clean(work)) {
		return nil
	}
	now := walkTree(root, work, false)
	var out []TreeChange
	for rel, h := range now {
		old, had := m.Files[rel]
		switch {
		case !had:
			out = append(out, TreeChange{Path: rel, New: true})
		case old != h:
			out = append(out, TreeChange{Path: rel, Before: old})
		}
	}
	for rel, h := range m.Files {
		if _, still := now[rel]; !still {
			out = append(out, TreeChange{Path: rel, Before: h, Deleted: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// TurnDependencies is the dependency fingerprint recorded at the start of the turn.
func TurnDependencies(root, session string, turn int) string {
	var m treeManifest
	if core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), &m) != nil {
		return ""
	}
	return m.Dependencies
}

// RefreshTurnDependencies rebases the turn's fingerprint after an install command the gate approved.
func RefreshTurnDependencies(root, session string, turn int) {
	p := filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json")
	var m treeManifest
	if core.ReadJSON(p, &m) != nil {
		return
	}
	m.Dependencies = DependencyFingerprint(m.Work)
	_ = core.WriteJSON(p, m)
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

// RestoreTurn rewrites work to the tree recorded at the start of a turn: files created later are
// removed and changed files come back from the blob store. This is what makes a shell deletion
// revertible, not just a write the tools checkpointed.
func RestoreTurn(root, work, session string, turn int) {
	var m treeManifest
	if core.ReadJSON(filepath.Join(treeDir(root), session, strconv.Itoa(turn)+".json"), &m) != nil {
		return
	}
	if !strings.EqualFold(filepath.Clean(m.Work), filepath.Clean(work)) {
		return
	}
	now := walkTree(root, work, false)
	for rel := range now {
		if _, had := m.Files[rel]; !had {
			_ = os.Remove(filepath.Join(work, filepath.FromSlash(rel)))
		}
	}
	for rel, hash := range m.Files {
		b, ok := ReadBlob(root, hash)
		if !ok {
			continue // too large to keep: the per-write checkpoint may still restore it
		}
		dst := filepath.Join(work, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, []byte(b), 0o644)
	}
}
