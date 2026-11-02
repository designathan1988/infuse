package agent

// The locks drawn from the project's instructions, and the loop that treats effects instead of causes.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// readCover is which lines of a file the agent has actually been shown. known holds the text of every
// line it was shown or wrote itself, so a change to the file (its own edit, a line-ending conversion)
// keeps what it already knows and asks only for the lines that are new to it.
type readCover struct {
	sum   string // hash of the content described (a time stamp misses two writes in one clock tick)
	seen  []bool
	known map[string]bool
}

// coverLine is a line as the agent knows it: line endings and indentation do not change what it says.
func coverLine(l string) string { return strings.TrimSpace(strings.TrimPrefix(l, "\uFEFF")) }

// refresh brings the coverage to the file content b; the caller holds s.Mu.
func (c *readCover) refresh(b []byte) {
	sum := fileHash(b)
	if c.sum == sum {
		return
	}
	lines := strings.Split(string(b), "\n")
	c.sum, c.seen = sum, make([]bool, len(lines))
	for i, l := range lines {
		k := coverLine(l)
		c.seen[i] = k == "" || c.known[k]
	}
}

// coverOf returns the coverage of abs, current with the file; the caller holds s.Mu.
func (s *Session) coverOf(abs string, create bool) *readCover {
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil
	}
	if s.cover == nil {
		s.cover = map[string]*readCover{}
	}
	c := s.cover[abs]
	if c == nil {
		if !create {
			return nil
		}
		c = &readCover{known: map[string]bool{}}
		s.cover[abs] = c
	}
	c.refresh(b)
	return c
}

// noteCoverage records that lines from..to (1-based) of abs were shown to the agent.
func (s *Session) noteCoverage(abs string, total, from, to int) {
	b, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	s.Mu.Lock()
	defer s.Mu.Unlock()
	c := s.coverOf(abs, true)
	if c == nil || len(c.seen) != total || len(lines) != total {
		return
	}
	for i := max(1, from); i <= min(to, total); i++ {
		c.seen[i-1] = true
		c.known[coverLine(lines[i-1])] = true
	}
}

// noteAuthored records the lines the agent itself wrote into abs: it knows them without reading them back.
func (s *Session) noteAuthored(abs string, texts ...string) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	c := s.coverOf(abs, true)
	if c == nil {
		return
	}
	for _, t := range texts {
		for _, l := range strings.Split(t, "\n") {
			c.known[coverLine(l)] = true
		}
	}
	c.sum = "" // seen is recomputed against the file with the new lines known
}

// writtenTexts is the text a Write, Edit or MultiEdit call put into its file.
func writtenTexts(input json.RawMessage) []string {
	var in struct {
		Content   string `json:"content"`
		NewString string `json:"new_string"`
		Source    string `json:"source"` // NotebookEdit
		Edits     []struct {
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(input, &in)
	out := []string{in.Content, in.NewString, in.Source}
	for _, e := range in.Edits {
		out = append(out, e.NewString)
	}
	return out
}

// readProgress is how many lines of abs the agent has seen, of how many, and where to continue.
func (s *Session) readProgress(abs string) (seen, total, next int) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	c := s.coverOf(abs, false)
	if c == nil {
		b, err := os.ReadFile(abs)
		if err != nil {
			return 0, 0, 0
		}
		return 0, strings.Count(string(b), "\n") + 1, 1
	}
	next = 0
	for i, ok := range c.seen {
		if ok {
			seen++
		} else if next == 0 {
			next = i + 1
		}
	}
	return seen, len(c.seen), next
}

// requiredReading returns why the agent may not act yet: documents the instructions point to that it has
// not read to the end (empty when all are read, or the project has none).
func (s *Session) requiredReading() Msg {
	r := loadProjectRules(s.Root)
	var missing []Msg
	for _, p := range r.Required {
		abs := filepath.Join(s.Root, filepath.FromSlash(p))
		seen, total, next := s.readProgress(abs)
		if total == 0 || seen >= total {
			continue
		}
		if seen == 0 {
			missing = append(missing, M("instr.not_read", "file", p))
		} else {
			missing = append(missing, M("instr.partly_read", "file", p, "seen", seen, "total", total, "next", next))
		}
	}
	if len(missing) == 0 {
		return Msg{}
	}
	return M("instr.required", "files", strings.Join(r.Files, ", "), "docs", missing)
}

// instructionWriteCheck: the instruction files are not edited; the forbidden references are not written.
func (s *Session) instructionWriteCheck(rel, before string) Msg {
	r := loadProjectRules(s.Root)
	if isInstructionFile(rel) {
		return M("instr.no_edit", "file", rel)
	}
	if ref := underReference(r, rel); ref != "" {
		return M("instr.reference", "ref", ref)
	}
	return Msg{}
}

// ---------------------------------------------------------------- the effects loop

var reCdPrefix = regexp.MustCompile(`^\s*cd\s+("[^"]*"|\S+)\s*(?:&&|;)\s*`)

// commandKey identifies "the same measurement" across runs: the command without cd and arguments' noise.
func commandKey(cmd string) string {
	c := strings.TrimSpace(reCdPrefix.ReplaceAllString(strings.TrimSpace(cmd), ""))
	if i := strings.IndexAny(c, "|>&;"); i > 0 {
		c = c[:i]
	}
	f := strings.Fields(c)
	if len(f) > 3 {
		f = f[:3]
	}
	return strings.Join(f, " ")
}

// recordRun counts a command run; a run after a change to the code is one more round of the loop.
func (s *Session) recordRun(cmd string, ok bool) {
	k := commandKey(cmd)
	if k == "" || helpCommand(cmd) {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	g := &s.gate
	g.ran = append(g.ran, ranCommand{normSpace(cmd), ok, g.writes})
	if g.runs == nil {
		g.runs = map[string]*runCount{}
	}
	rc := g.runs[k]
	if rc == nil {
		rc = &runCount{}
		g.runs[k] = rc
	}
	if rc.total > 0 && g.writes > rc.writesAt {
		rc.rounds++
	}
	rc.total++
	rc.writesAt = g.writes
	if rc.rounds >= 3 && g.loopKey == "" {
		g.loopKey, g.loopSince = k, time.Now()
		g.diagnosed, g.redoWhy = false, M("redo.loop", "command", k, "rounds", rc.rounds)
	}
	// a check the project declares, which passed earlier in this turn, fails after a change: a regression
	if !ok {
		for _, want := range loadProjectRules(s.Root).AfterChange {
			w := normSpace(want)
			if !strings.Contains(normSpace(cmd), w) {
				continue
			}
			for _, prev := range g.ran[:len(g.ran)-1] {
				if prev.ok && strings.Contains(prev.cmd, w) && prev.afterWrites < g.writes {
					g.diagnosed, g.redoWhy = false, M("redo.regression", "command", want)
				}
			}
		}
	}
}

type runCount struct{ total, rounds, writesAt int }

type ranCommand struct {
	cmd         string
	ok          bool
	afterWrites int // changes made before it ran
}

// missingVerification lists the commands the instructions order after changes that did not run, with
// success, after this turn's last change (empty when none is missing or nothing changed).
func (s *Session) missingVerification() Msg {
	r := loadProjectRules(s.Root)
	s.Mu.Lock()
	writes, ran := s.gate.writes, append([]ranCommand{}, s.gate.ran...)
	s.Mu.Unlock()
	if writes == 0 || len(r.AfterChange) == 0 {
		return Msg{}
	}
	var missing []string
	for _, want := range r.AfterChange {
		w := normSpace(want)
		done := false
		for _, c := range ran {
			if c.ok && c.afterWrites == writes && strings.Contains(c.cmd, w) {
				done = true
			}
		}
		if !done {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return Msg{}
	}
	return M("instr.verify_missing", "commands", strings.Join(missing, " · "))
}

// noteWrite and noteInvestigation feed the loop detector.
func (s *Session) noteWrite() { s.Mu.Lock(); s.gate.writes++; s.Mu.Unlock() }

// investigates reports whether a call studies the cause: project documents, the web, a research subagent.
func investigates(root, name, path string) bool {
	switch name {
	case "WebSearch", "WebFetch", "Skill":
		return true
	case "Read":
		p := strings.ToLower(filepath.ToSlash(path))
		if strings.HasSuffix(p, ".md") || strings.Contains(p, "/docs/") || strings.HasPrefix(p, "docs/") {
			return true
		}
		for _, req := range loadProjectRules(root).Required {
			if strings.HasSuffix(p, strings.ToLower(req)) {
				return true
			}
		}
	}
	return false
}

// loopCheck refuses the next change or command while the agent is in a measure-and-tweak loop.
func (s *Session) loopCheck(name, cat, path string, isAgentExplore bool) Msg {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	g := &s.gate
	if g.loopKey == "" {
		return Msg{}
	}
	if isAgentExplore || investigates(s.Root, name, path) {
		g.studied++
		if g.studied >= 2 { // it went to the sources: the loop is over; counting starts again
			g.loopKey, g.studied, g.runs = "", 0, nil
		}
		return Msg{}
	}
	if cat != "write" && cat != "exec" {
		return Msg{}
	}
	docs := strings.Join(loadProjectRules(s.Root).Required, ", ")
	if docs == "" {
		return M("instr.loop", "command", g.loopKey)
	}
	return M("instr.loop_docs", "command", g.loopKey, "docs", docs)
}

// instructionsPrompt tells the agent what infuse extracted from the instructions and will enforce.
func instructionsPrompt(root string) string {
	r := loadProjectRules(root)
	if len(r.Files) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## What infuse enforces from the project's instructions\n")
	if len(r.Missing) > 0 {
		b.WriteString("- " + strings.Join(r.Missing, ", ") + " was deleted from the working folder; the git version, read above, is the one that applies. Do not delete instructions.\n")
	}
	if len(r.Required) > 0 {
		b.WriteString("- Required reading, in full, before changing files or running commands: " + strings.Join(r.Required, ", ") + ".\n")
	}
	if len(r.Reference) > 0 {
		b.WriteString("- Read-only reference; copying code from it or writing in it is forbidden: " + strings.Join(r.Reference, ", ") + ".\n")
	}
	if len(r.Commands) > 0 {
		b.WriteString("- The project's verification and measurement (use these, not tools of your own): " + strings.Join(r.Commands, " · ") + ".\n")
	}
	b.WriteString("- Repeating a measurement after each tweak, without researching the cause, blocks the turn until you study the documentation.\n")
	return b.String()
}

// noteFileState records the content a write left; returning a file to a content it had earlier in the
// turn (other than the one just before) is the agent undoing its own work: the approach was wrong.
func (s *Session) noteFileState(path string) {
	if path == "" {
		return
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.Cwd, abs)
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return
	}
	h := fileHash(b)
	rel := relTo(s.Root, abs)
	s.Mu.Lock()
	defer s.Mu.Unlock()
	g := &s.gate
	if g.fileHashes == nil {
		g.fileHashes = map[string][]string{}
	}
	hist := g.fileHashes[rel]
	for i, old := range hist {
		if old == h && i < len(hist)-1 {
			g.diagnosed, g.redoWhy = false, M("redo.revert", "file", rel)
			break
		}
	}
	g.fileHashes[rel] = append(hist, h)
}

// noteFileStateBefore records a file's content before its first change in the turn.
func (s *Session) noteFileStateBefore(path string) {
	if path == "" {
		return
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.Cwd, abs)
	}
	rel := relTo(s.Root, abs)
	s.Mu.Lock()
	_, known := s.gate.fileHashes[rel]
	s.Mu.Unlock()
	if known {
		return
	}
	b, err := os.ReadFile(abs)
	h := "absent"
	if err == nil {
		h = fileHash(b)
	}
	s.Mu.Lock()
	if s.gate.fileHashes == nil {
		s.gate.fileHashes = map[string][]string{}
	}
	s.gate.fileHashes[rel] = []string{h}
	s.Mu.Unlock()
}
