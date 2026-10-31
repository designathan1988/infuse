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

// sessionBaselines maps each file the session changed to its content before the first change ("" and
// isNew for files it created).
func (s *Session) sessionBaselines() map[string]struct {
	content string
	isNew   bool
} {
	s.mu.Lock()
	turns := make([]int, 0, len(s.Checkpoints))
	for t := range s.Checkpoints {
		turns = append(turns, t)
	}
	sort.Ints(turns)
	cps := map[int][]CheckpointFile{}
	for _, t := range turns {
		cps[t] = append([]CheckpointFile{}, s.Checkpoints[t]...)
	}
	s.mu.Unlock()
	out := map[string]struct {
		content string
		isNew   bool
	}{}
	for _, t := range turns {
		for _, c := range cps[t] {
			if _, seen := out[c.Path]; seen {
				continue
			}
			if c.New {
				out[c.Path] = struct {
					content string
					isNew   bool
				}{"", true}
				continue
			}
			b, _ := os.ReadFile(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t), filepath.FromSlash(c.Path)))
			out[c.Path] = struct {
				content string
				isNew   bool
			}{string(b), false}
		}
	}
	return out
}

// LocalChanges lists what the session changed in a folder without git, with line counts.
func (s *Session) LocalChanges() map[string]any {
	files := []fileChange{}
	add, rem := 0, 0
	for p, base := range s.sessionBaselines() {
		now, err := os.ReadFile(filepath.Join(s.workRoot(), filepath.FromSlash(p)))
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

// LocalDiff is the unified diff of one file against its session baseline.
func (s *Session) LocalDiff(p string) string {
	base, ok := s.sessionBaselines()[p]
	if !ok {
		return ""
	}
	now, _ := os.ReadFile(filepath.Join(s.workRoot(), filepath.FromSlash(p)))
	d, _, _ := unifiedDiff(p, splitLines(base.content), splitLines(string(now)))
	return d
}

// LocalDiscard puts a file back as it was before the session changed it.
func (s *Session) LocalDiscard(p string) error {
	base, ok := s.sessionBaselines()[p]
	if !ok {
		return fmt.Errorf("arquivo sem alteração desta sessão")
	}
	abs := filepath.Join(s.workRoot(), filepath.FromSlash(p))
	if !within(s.workRoot(), abs) {
		return fmt.Errorf("fora do projeto")
	}
	if base.isNew {
		return os.Remove(abs)
	}
	return os.WriteFile(abs, []byte(base.content), 0o644)
}
