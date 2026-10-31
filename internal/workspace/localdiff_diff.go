package workspace

// Changes without git: the baseline of each file is the copy infuse kept before the session's first
// change to it (the checkpoints), or nothing for a file the session created. Line counts and the diff
// view come from comparing that baseline with the file now, so a plain local folder shows the same
// "+added −removed" as a repository.

import (
	"fmt"
	"strings"
)

// diffOp is one line of an edit script: ' ' keep, '-' remove, '+' add.
type diffOp struct {
	op   byte
	line string
}

// lineDiff returns the edit script between a and b as ops: ' ' keep, '-' remove, '+' add.
func lineDiff(a, b []string) []diffOp {
	pre, suf := commonEdges(a, b)
	var out []diffOp
	for i := 0; i < pre; i++ {
		out = append(out, diffOp{' ', a[i]})
	}
	out = append(out, middleOps(a[pre:len(a)-suf], b[pre:len(b)-suf])...)
	for i := len(a) - suf; i < len(a); i++ {
		out = append(out, diffOp{' ', a[i]})
	}
	return out
}

// commonEdges trims the common head and tail: most edits touch a small part of a file.
func commonEdges(a, b []string) (pre, suf int) {
	n, m := len(a), len(b)
	for pre < n && pre < m && a[pre] == b[pre] {
		pre++
	}
	for suf < n-pre && suf < m-pre && a[n-1-suf] == b[m-1-suf] {
		suf++
	}
	return pre, suf
}

// middleOps is the edit script of the changed middle: a longest common subsequence, or a whole
// replacement when the table would be too large.
func middleOps(ma, mb []string) []diffOp {
	var out []diffOp
	if len(ma)*len(mb) > 6_000_000 { // too large for the table: everything in the middle changed
		for _, l := range ma {
			out = append(out, diffOp{'-', l})
		}
		for _, l := range mb {
			out = append(out, diffOp{'+', l})
		}
		return out
	}
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
			out = append(out, diffOp{' ', ma[i]})
			i, j = i+1, j+1
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			out = append(out, diffOp{'-', ma[i]})
			i++
		default:
			out = append(out, diffOp{'+', mb[j]})
			j++
		}
	}
	for ; i < len(ma); i++ {
		out = append(out, diffOp{'-', ma[i]})
	}
	for ; j < len(mb); j++ {
		out = append(out, diffOp{'+', mb[j]})
	}
	return out
}

// countEdits counts the added and removed lines in an edit script.
func countEdits(ops []diffOp) (added, removed int) {
	for _, o := range ops {
		if o.op == '+' {
			added++
		} else if o.op == '-' {
			removed++
		}
	}
	return added, removed
}

// unifiedDiff renders the ops as a unified diff with three lines of context.
func unifiedDiff(path string, a, b []string) (string, int, int) {
	ops := lineDiff(a, b)
	added, removed := countEdits(ops)
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
		start, stop := hunkRange(ops, i, ctx)
		oa, la, ob, lb := hunkHeader(ops, start, stop)
		if la == 0 { // unified diff: an empty side starts at the line before it
			oa--
		}
		if lb == 0 {
			ob--
		}
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", oa, la, ob, lb)
		writeHunk(&sb, ops, start, stop)
		i = stop
	}
	return sb.String(), added, removed
}

// hunkRange gives the bounds of the hunk starting at the change i: at most ctx lines before it,
// extending while changes are within 2*ctx of each other, and ctx lines after.
func hunkRange(ops []diffOp, i, ctx int) (start, stop int) {
	start = max(0, i-ctx)
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
	stop = min(len(ops), end+ctx)
	return start, stop
}

// hunkHeader counts the hunk's lines on each side and the line numbers it starts at (1-based).
func hunkHeader(ops []diffOp, start, stop int) (oa, la, ob, lb int) {
	oa, ob = 1, 1 // line numbers of the hunk start
	for k := 0; k < start; k++ {
		if ops[k].op != '+' {
			oa++
		}
		if ops[k].op != '-' {
			ob++
		}
	}
	for k := start; k < stop; k++ {
		if ops[k].op != '+' {
			la++
		}
		if ops[k].op != '-' {
			lb++
		}
	}
	return oa, la, ob, lb
}

// writeHunk writes the hunk's lines, each with its prefix and newline.
func writeHunk(sb *strings.Builder, ops []diffOp, start, stop int) {
	for k := start; k < stop; k++ {
		sb.WriteByte(ops[k].op)
		sb.WriteString(ops[k].line)
		sb.WriteByte('\n')
	}
}
