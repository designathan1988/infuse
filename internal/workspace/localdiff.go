package workspace

import (
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Baselines from session checkpoints and turn snapshots.

type baseline struct {
	content string
	isNew   bool
	hash    string // a turn snapshot's entry: the content is in the blob store
}

// LiveSession is the checkpoint state of a session still running in this process.
type LiveSession struct {
	ID          string
	Created     int64
	Checkpoints map[int][]core.CheckpointFile
}

// SnapshotSource supplies the sessions running right now (their checkpoints are not on disk yet).
type SnapshotSource interface {
	LiveSessions(root string) []LiveSession
}

func sessionsDir(root string) string {
	return filepath.Join(root, core.InfuseDirName, "sessions")
}

// projectBaselines maps each file any session of the project changed to its content before the first
// change (earliest session, earliest turn); "" and isNew for files a session created.
func projectBaselines(root string, src SnapshotSource, keep func(session string, created int64, turn int) bool) map[string]baseline {
	entries, _ := os.ReadDir(sessionsDir(root))
	type sess struct {
		id      string
		created int64
		cps     map[int][]core.CheckpointFile
	}
	var list []sess
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var d struct {
			ID          string                        `json:"id"`
			Created     int64                         `json:"created"`
			Checkpoints map[int][]core.CheckpointFile `json:"checkpoints"`
		}
		if core.ReadJSON(filepath.Join(sessionsDir(root), e.Name()), &d) == nil && len(d.Checkpoints) > 0 {
			list = append(list, sess{d.ID, d.Created, d.Checkpoints})
		}
	}
	// a session running now has checkpoints not saved yet
	if src != nil {
		for _, live := range src.LiveSessions(root) {
			replaced := false
			for i := range list {
				if list[i].id == live.ID {
					list[i].cps, replaced = live.Checkpoints, true
				}
			}
			if !replaced && len(live.Checkpoints) > 0 {
				list = append(list, sess{live.ID, live.Created, live.Checkpoints})
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].created < list[j].created })
	out := map[string]baseline{}
	for _, se := range list {
		turns := make([]int, 0, len(se.cps))
		for t := range se.cps {
			turns = append(turns, t)
		}
		sort.Ints(turns)
		for _, t := range turns {
			if keep != nil && !keep(se.id, se.created, t) {
				continue
			}
			for _, c := range se.cps[t] {
				if _, seen := out[c.Path]; seen {
					continue
				}
				if c.New {
					out[c.Path] = baseline{isNew: true}
					continue
				}
				b, err := os.ReadFile(filepath.Join(root, core.InfuseDirName, "checkpoints", se.id, fmt.Sprint(t), filepath.FromSlash(c.Path)))
				if err != nil {
					continue // the copy is gone (rewound): this change is not measurable
				}
				out[c.Path] = baseline{content: string(b)}
			}
		}
	}
	return out
}

// localBase is what each file was at the start of the scope: "all" (since infuse first saw the folder)
// or "turn" (since the start of the session's latest turn). complete reports that the base covers the
// whole tree (a turn snapshot exists), so any file outside it is new.
func localBase(root, work, session, scope string, src SnapshotSource) (base map[string]baseline, complete bool, since int64) {
	base = map[string]baseline{}
	if scope == "turn" && session != "" {
		if ms := Manifests(root, work, session); len(ms) > 0 {
			m := ms[len(ms)-1]
			for p, h := range m.Files {
				base[p] = baseline{hash: h}
			}
			return base, true, m.At
		}
		// a session from before turn snapshots: the copies kept in its latest turn
		var last int
		_ = core.ReadJSON(filepath.Join(sessionsDir(root), session+".json"), &struct {
			Turn *int `json:"turn"`
		}{&last})
		return projectBaselines(root, src, func(id string, _ int64, t int) bool { return id == session && t == last }), false, 0
	}
	ms := Manifests(root, work, "")
	var first *treeManifest
	if len(ms) > 0 {
		first = &ms[0]
	}
	// copies kept by sessions older than the first snapshot come first: they are earlier
	base = projectBaselines(root, src, func(_ string, created int64, _ int) bool { return first == nil || created < first.At })
	if first == nil {
		return base, false, earliestSession(root)
	}
	for p, h := range first.Files {
		if _, ok := base[p]; !ok {
			base[p] = baseline{hash: h}
		}
	}
	return base, true, first.At
}

// earliestSession is the creation time of the project's first session (0 when none).
func earliestSession(root string) int64 {
	entries, _ := os.ReadDir(sessionsDir(root))
	var first int64
	for _, e := range entries {
		var d struct {
			Created int64 `json:"created"`
		}
		if strings.HasSuffix(e.Name(), ".json") && core.ReadJSON(filepath.Join(sessionsDir(root), e.Name()), &d) == nil && d.Created > 0 && (first == 0 || d.Created < first) {
			first = d.Created
		}
	}
	return first
}

// text is the baseline's content (from the blob store for snapshot entries).
func (b baseline) text(root string) (string, bool) {
	if b.hash != "" {
		return ReadBlob(root, b.hash)
	}
	return b.content, true
}

// localFileChanges compares the scope's base with the folder now.
func localFileChanges(root, work, session, scope string, src SnapshotSource) ([]FileChange, map[string]baseline) {
	base, complete, since := localBase(root, work, session, scope, src)
	now := walkTree(root, work, false)
	paths := map[string]bool{}
	for p := range base {
		paths[p] = true
	}
	for p := range now {
		if _, known := base[p]; known {
			continue
		}
		if complete {
			paths[p] = true
			base[p] = baseline{isNew: true}
			continue
		}
		// no snapshot yet: a file written after the project's first session is a change of that time
		if since > 0 {
			if fi, err := os.Stat(filepath.Join(work, filepath.FromSlash(p))); err == nil && fi.ModTime().UnixMilli() > since-2000 { // ids taken in the same millisecond move Created slightly ahead
				paths[p] = true
				base[p] = baseline{isNew: true}
			}
		}
	}
	var files []FileChange
	for p := range paths {
		b := base[p]
		cur, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(p)))
		gone := err != nil
		if gone && b.isNew {
			continue // created and removed again
		}
		if !gone && b.hash != "" && b.hash == hashBytes(cur) {
			continue
		}
		before, ok := b.text(root)
		if !ok {
			continue // the stored copy is gone: not measurable
		}
		st := "M"
		switch {
		case gone:
			st = "D"
		case b.isNew:
			st = "A"
		}
		_, a, r := unifiedDiff(p, core.SplitLines(before), core.SplitLines(string(cur)))
		if a+r == 0 && st == "M" {
			continue
		}
		files = append(files, FileChange{Path: p, Status: st, Added: a, Removed: r})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, base
}

// LocalChanges lists the changes of a folder without git, with line counts, for the scope.
