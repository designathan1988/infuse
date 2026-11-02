package filter

// Reads of infuse's hidden records (other sessions, the journal, snapshots…). The OS sandbox contains
// writes only: on every platform a contained process can still read the whole project, so nothing
// below the parser stops `cat .infuse/sessions/x.json`. The path tools are exact (resolvePath plus
// HiddenAt); the shell is best effort: it refuses the readers it recognizes when they name a hidden
// record, and a recursive reader started above .infuse (grep -r x .) or a program that opens the file
// itself is not seen. The oracle stays readable through the shell: the agent sees the criteria.

import (
	"infuse/internal/core"
	"infuse/internal/journal"
	"regexp"
	"strings"
)

var reInputRedir = regexp.MustCompile(`(?:^|[^<])<\s*("[^"]*"|'[^']*'|[^\s;&|<>()]+)`)

// readingCommands hand the content of the files they name to the agent, or to another file.
var readingCommands = map[string]bool{
	"cat": true, "tac": true, "nl": true, "head": true, "tail": true, "less": true, "more": true, "bat": true,
	"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "awk": true, "gawk": true, "sed": true,
	"sort": true, "uniq": true, "cut": true, "wc": true, "strings": true, "xxd": true, "od": true, "hexdump": true,
	"base64": true, "base32": true, "diff": true, "cmp": true, "comm": true, "paste": true, "dd": true,
	"tar": true, "zip": true, "7z": true, "cp": true, "rsync": true, "scp": true, "tee": false,
	"type": true, "gc": true, "get-content": true, "findstr": true, "select-string": true, "sls": true,
}

// patternFirst are the readers whose first positional is a pattern or a program, not a file.
var patternFirst = map[string]bool{"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "awk": true, "gawk": true, "sed": true, "findstr": true}

// HiddenAt reports whether abs is one of infuse's hidden records, from any starting point, and returns
// the record's .infuse-relative name. The roots the reader works in are cut first, so a worktree kept
// under .infuse/worktrees is plain project text to its own session; what remains is judged from its
// last .infuse component on, which is where any project's records are, however they were reached.
func HiddenAt(abs string, roots ...string) (string, bool) {
	p := abs
	for _, r := range roots {
		if core.Within(r, abs) {
			p = core.RelTo(r, abs)
			break
		}
	}
	parts := strings.Split(strings.ReplaceAll(p, `\`, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if foldPath(parts[i]) == core.InfuseDirName {
			tail := strings.Join(parts[i:], "/")
			return tail, HiddenInternal(tail)
		}
	}
	return "", false
}

// readsHidden reports whether reading p reaches a hidden record the shell must not hand over.
func (g *Guard) readsHidden(dir, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || strings.HasPrefix(p, "-") {
		return "", false
	}
	for _, abs := range pathSpellings(CanonPath(dir, p)) {
		rel, hidden := HiddenAt(abs, g.roots...)
		if hidden && !criteria(rel) {
			return rel, true
		}
	}
	return "", false
}

// criteria: the oracle is hidden from the listing tools but open to read, by design (SPEC §5).
func criteria(rel string) bool {
	rel = foldPath(rel)
	return rel == oracleRel || strings.HasPrefix(rel, oracleRel+"/")
}

// blockRead records a refused read; the error carries the message (msgOf).
func (g *Guard) blockRead(rel string) error {
	detail := core.M("guard.hidden_read").String()
	journal.Journal(g.Root, journal.JournalEntry{Kind: "blocked", Path: rel, Detail: detail})
	journal.Lesson(g.Root, "hidden-record-read", detail+": "+rel)
	return core.ErrMsg(core.M("path.internal", "path", rel))
}

// checkHiddenRead refuses a reader that names a hidden record.
func (g *Guard) checkHiddenRead(head string, args []string, dir string) error {
	if !readingCommands[head] && !(head == "find" && findRuns(args)) {
		return nil
	}
	for _, a := range readArgs(head, args) {
		for _, cand := range strings.Split(a, "=") {
			if rel, ok := g.readsHidden(dir, cand); ok {
				return g.blockRead(rel)
			}
		}
	}
	return nil
}

// readArgs drops the pattern of the readers that take one, so a search FOR the text ".infuse/sessions"
// is not a read OF it.
func readArgs(head string, args []string) []string {
	if !patternFirst[head] {
		return args
	}
	out := make([]string, 0, len(args))
	skipped := false
	for _, a := range args {
		if a == "-e" || a == "-f" || a == "--regexp" || a == "--file" {
			return args // the pattern came as an option: every positional is a file
		}
		if !skipped && !strings.HasPrefix(a, "-") {
			skipped = true
			continue
		}
		out = append(out, a)
	}
	return out
}

func findRuns(args []string) bool {
	for _, a := range args {
		if a == "-exec" || a == "-execdir" || a == "-ok" || a == "-okdir" {
			return true
		}
	}
	return false
}

// checkInputRedirects refuses `< record`, which reads for any command.
func (g *Guard) checkInputRedirects(text, dir string) error {
	for _, m := range reInputRedir.FindAllStringSubmatch(text, -1) {
		if rel, ok := g.readsHidden(dir, m[1]); ok {
			return g.blockRead(rel)
		}
	}
	return nil
}
