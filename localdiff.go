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

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
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
}

// projectBaselines maps each file any session of the project changed to its content before the first
// change (earliest session, earliest turn); "" and isNew for files a session created.
func projectBaselines(root string) map[string]baseline {
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
			for _, c := range se.cps[t] {
				if _, seen := out[c.Path]; seen {
					continue
				}
				if c.New {
					out[c.Path] = baseline{"", true}
					continue
				}
				b, err := os.ReadFile(filepath.Join(root, infuseDirName, "checkpoints", se.id, fmt.Sprint(t), filepath.FromSlash(c.Path)))
				if err != nil {
					continue // the copy is gone (rewound): this change is not measurable
				}
				out[c.Path] = baseline{string(b), false}
			}
		}
	}
	return out
}

// LocalChanges lists what infuse's sessions changed in a folder without git, with line counts.
func LocalChanges(root, work string) map[string]any {
	files := []fileChange{}
	add, rem := 0, 0
	for p, base := range projectBaselines(root) {
		now, err := os.ReadFile(filepath.Join(work, filepath.FromSlash(p)))
		st := "M"
		switch {
		case err != nil && base.isNew:
			continue // created and removed again
		case err != nil:
			st = "D"
		case base.isNew:
			st = "A"
		}
		_, a, r := unifiedDiff(p, splitLines(base.content), splitLines(string(now)))
		if a+r == 0 && st == "M" {
			continue
		}
		files = append(files, fileChange{Path: p, Status: st, Added: a, Removed: r})
		add += a
		rem += r
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return map[string]any{"isGit": false, "local": true, "files": files, "added": add, "removed": rem}
}

// LocalDiff is the unified diff of one file against its baseline.
func LocalDiff(root, work, p string) string {
	base, ok := projectBaselines(root)[p]
	if !ok {
		return ""
	}
	now, _ := os.ReadFile(filepath.Join(work, filepath.FromSlash(p)))
	d, _, _ := unifiedDiff(p, splitLines(base.content), splitLines(string(now)))
	return d
}

// LocalDiscard puts a file back as it was before infuse's sessions changed it.
func LocalDiscard(root, work, p string) error {
	base, ok := projectBaselines(root)[p]
	if !ok {
		return fmt.Errorf("arquivo sem alteração registrada")
	}
	abs := filepath.Join(work, filepath.FromSlash(p))
	if !within(work, abs) {
		return fmt.Errorf("fora do projeto")
	}
	if base.isNew {
		return os.Remove(abs)
	}
	return os.WriteFile(abs, []byte(base.content), 0o644)
}
