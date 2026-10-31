package main

// Changes without git: the baseline of each file is the copy infuse kept before the session's first
// change to it (the checkpoints), or nothing for a file the session created. Line counts and the diff
// view come from comparing that baseline with the file now, so a plain local folder shows the same
// "+added −removed" as a repository.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// lineDiff returns the edit script between a and b as ops: ' ' keep, '-' remove, '+' add.
func lineDiff(a, b []string) []struct {
	op   byte
	line string
} {
	n, m := len(a), len(b)
	type op = struct {
		op   byte
		line string
	}
	// trim the common head and tail: most edits touch a small part of a file
	pre := 0
	for pre < n && pre < m && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < n-pre && suf < m-pre && a[n-1-suf] == b[m-1-suf] {
		suf++
	}
	var out []op
	for i := 0; i < pre; i++ {
		out = append(out, op{' ', a[i]})
	}
	ma, mb := a[pre:n-suf], b[pre:m-suf]
	if len(ma)*len(mb) > 6_000_000 { // too large for the table: everything in the middle changed
		for _, l := range ma {
			out = append(out, op{'-', l})
		}
		for _, l := range mb {
			out = append(out, op{'+', l})
		}
	} else {
		// longest common subsequence, then walk it
		w := len(mb) + 1
		lcs := make([]int32, (len(ma)+1)*w)
		for i := len(ma) - 1; i >= 0; i-- {
			for j := len(mb) - 1; j >= 0; j-- {
				if ma[i] == mb[j] {
					lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
				} else if lcs[(i+1)*w+j] >= lcs[i*w+j+1] {
					lcs[i*w+j] = lcs[(i+1)*w+j]
				} else {
					lcs[i*w+j] = lcs[i*w+j+1]
				}
			}
		}
		i, j := 0, 0
		for i < len(ma) && j < len(mb) {
			switch {
			case ma[i] == mb[j]:
				out = append(out, op{' ', ma[i]})
				i, j = i+1, j+1
			case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
				out = append(out, op{'-', ma[i]})
				i++
			default:
				out = append(out, op{'+', mb[j]})
				j++
			}
		}
		for ; i < len(ma); i++ {
			out = append(out, op{'-', ma[i]})
		}
		for ; j < len(mb); j++ {
			out = append(out, op{'+', mb[j]})
		}
	}
	for i := n - suf; i < n; i++ {
		out = append(out, op{' ', a[i]})
	}
	return out
}


// unifiedDiff renders the ops as a unified diff with three lines of context.
func unifiedDiff(path string, a, b []string) (string, int, int) {
	ops := lineDiff(a, b)
	added, removed := 0, 0
	for _, o := range ops {
		if o.op == '+' {
			added++
		} else if o.op == '-' {
			removed++
		}
	}
	if added+removed == 0 {
		return "", 0, 0
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", path, path)
	const ctx = 3
	for i := 0; i < len(ops); {
		if ops[i].op == ' ' {
			i++
			continue
		}
		start := max(0, i-ctx)
		end := i
		for end < len(ops) { // extend while changes are within 2*ctx of each other
			if ops[end].op != ' ' {
				end++
				continue
			}
			k := end
			for k < len(ops) && ops[k].op == ' ' && k-end < 2*ctx {
				k++
			}
			if k < len(ops) && ops[k].op != ' ' {
				end = k
				continue
			}
			break
		}
		stop := min(len(ops), end+ctx)
		oa, ob := 1, 1 // line numbers of the hunk start
		for k := 0; k < start; k++ {
			if ops[k].op != '+' {
				oa++
			}
			if ops[k].op != '-' {
				ob++
			}
		}
		la, lb := 0, 0
		for k := start; k < stop; k++ {
			if ops[k].op != '+' {
				la++
			}
			if ops[k].op != '-' {
				lb++
			}
		}
		if la == 0 { // unified diff: an empty side starts at the line before it
			oa--
		}
		if lb == 0 {
			ob--
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", oa, la, ob, lb)
		for k := start; k < stop; k++ {
			sb.WriteByte(ops[k].op)
			sb.WriteString(ops[k].line)
			sb.WriteByte('\n')
		}
		i = stop
	}
	return sb.String(), added, removed
}

type baseline struct {
	content string
	isNew   bool
	hash    string // a turn snapshot's entry: the content is in the blob store
}

// projectBaselines maps each file any session of the project changed to its content before the first
// change (earliest session, earliest turn); "" and isNew for files a session created.
func projectBaselines(root string, keep func(session string, created int64, turn int) bool) map[string]baseline {
	entries, _ := os.ReadDir(sessionsDir(root))
	type sess struct {
		id      string
		created int64
		cps     map[int][]CheckpointFile
	}
	var list []sess
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var d struct {
			ID          string                   `json:"id"`
			Created     int64                    `json:"created"`
			Checkpoints map[int][]CheckpointFile `json:"checkpoints"`
		}
		if readJSON(filepath.Join(sessionsDir(root), e.Name()), &d) == nil && len(d.Checkpoints) > 0 {
			list = append(list, sess{d.ID, d.Created, d.Checkpoints})
		}
	}
	// a session running now has checkpoints not saved yet
	sessionsMu.Lock()
	for _, live := range sessions {
		if live.Root != root {
			continue
		}
		live.mu.Lock()
		cps := map[int][]CheckpointFile{}
		for t, c := range live.Checkpoints {
			cps[t] = append([]CheckpointFile{}, c...)
		}
		id, created := live.ID, live.Created
		live.mu.Unlock()
		replaced := false
		for i := range list {
			if list[i].id == id {
				list[i].cps, replaced = cps, true
			}
		}
		if !replaced && len(cps) > 0 {
			list = append(list, sess{id, created, cps})
		}
	}
	sessionsMu.Unlock()
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
				b, err := os.ReadFile(filepath.Join(root, infuseDirName, "checkpoints", se.id, fmt.Sprint(t), filepath.FromSlash(c.Path)))
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
func localBase(root, work, session, scope string) (base map[string]baseline, complete bool, since int64) {
	base = map[string]baseline{}
	if scope == "turn" && session != "" {
		if ms := manifests(root, work, session); len(ms) > 0 {
			m := ms[len(ms)-1]
			for p, h := range m.Files {
				base[p] = baseline{hash: h}
			}
			return base, true, m.At
		}
		// a session from before turn snapshots: the copies kept in its latest turn
		var last int
		_ = readJSON(filepath.Join(sessionsDir(root), session+".json"), &struct {
			Turn *int `json:"turn"`
		}{&last})
		return projectBaselines(root, func(id string, _ int64, t int) bool { return id == session && t == last }), false, 0
	}
	ms := manifests(root, work, "")
	var first *treeManifest
	if len(ms) > 0 {
		first = &ms[0]
	}
	// copies kept by sessions older than the first snapshot come first: they are earlier
	base = projectBaselines(root, func(_ string, created int64, _ int) bool { return first == nil || created < first.At })
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
		if strings.HasSuffix(e.Name(), ".json") && readJSON(filepath.Join(sessionsDir(root), e.Name()), &d) == nil && d.Created > 0 && (first == 0 || d.Created < first) {
			first = d.Created
		}
	}
	return first
}

// text is the baseline's content (from the blob store for snapshot entries).
func (b baseline) text(root string) (string, bool) {
	if b.hash != "" {
		return readBlob(root, b.hash)
	}
	return b.content, true
}

// localFileChanges compares the scope's base with the folder now.
func localFileChanges(root, work, session, scope string) ([]fileChange, map[string]baseline) {
	base, complete, since := localBase(root, work, session, scope)
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
	var files []fileChange
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
		_, a, r := unifiedDiff(p, splitLines(before), splitLines(string(cur)))
		if a+r == 0 && st == "M" {
			continue
		}
		files = append(files, fileChange{Path: p, Status: st, Added: a, Removed: r})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, base
}

// LocalChanges lists the changes of a folder without git, with line counts, for the scope.
func LocalChanges(root, work, session, scope string) map[string]any {
	files, _ := localFileChanges(root, work, session, scope)
	if files == nil {
		files = []fileChange{}
	}
	add, rem := 0, 0
	for _, f := range files {
		add += f.Added
		rem += f.Removed
	}
	return map[string]any{"isGit": false, "local": true, "scope": firstNonEmpty(scope, "all"), "files": files, "added": add, "removed": rem}
}

// LocalDiff is the unified diff of one file against the scope's base.
func LocalDiff(root, work, session, scope, p string) string {
	_, base := localFileChanges(root, work, session, scope)
	b, ok := base[p]
	if !ok {
		return ""
	}
	before, _ := b.text(root)
	now, _ := os.ReadFile(filepath.Join(work, filepath.FromSlash(p)))
	d, _, _ := unifiedDiff(p, splitLines(before), splitLines(string(now)))
	return d
}

// LocalDiscard puts a file back as it was at the start of the scope: rewritten, recreated or removed.
func LocalDiscard(root, work, session, scope, p string) error {
	_, base := localFileChanges(root, work, session, scope)
	b, ok := base[p]
	if !ok {
		return fmt.Errorf("arquivo sem alteração registrada")
	}
	abs := filepath.Join(work, filepath.FromSlash(p))
	if !within(work, abs) {
		return fmt.Errorf("fora do projeto")
	}
	if b.isNew {
		return os.Remove(abs)
	}
	before, ok := b.text(root)
	if !ok {
		return fmt.Errorf("a cópia anterior não existe mais")
	}
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	return os.WriteFile(abs, []byte(before), 0o644)
}
