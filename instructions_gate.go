package main

// The locks drawn from the project's instructions, and the loop that treats effects instead of causes.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// readCover is which lines of a file the agent has actually been shown.
type readCover struct {
	mod  int64
	seen []bool
}

// noteCoverage records that lines from..to (1-based) of abs were shown to the agent.
func (s *Session) noteCoverage(abs string, total, from, to int) {
	fi, err := os.Stat(abs)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cover == nil {
		s.cover = map[string]*readCover{}
	}
	c := s.cover[abs]
	if c == nil || c.mod != fi.ModTime().UnixNano() || len(c.seen) != total {
		c = &readCover{mod: fi.ModTime().UnixNano(), seen: make([]bool, total)}
		s.cover[abs] = c
	}
	for i := max(1, from); i <= min(to, total); i++ {
		c.seen[i-1] = true
	}
}

// readProgress is how many lines of abs the agent has seen, of how many, and where to continue.
func (s *Session) readProgress(abs string) (seen, total, next int) {
	fi, err := os.Stat(abs)
	if err != nil {
		return 0, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cover[abs]
	if c == nil || c.mod != fi.ModTime().UnixNano() {
		b, _ := os.ReadFile(abs)
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
// not read to the end ("" when all are read, or the project has none).
func (s *Session) requiredReading() string {
	r := loadProjectRules(s.Root)
	var missing []string
	for _, p := range r.Required {
		abs := filepath.Join(s.Root, filepath.FromSlash(p))
		seen, total, next := s.readProgress(abs)
		if total == 0 || seen >= total {
			continue
		}
		if seen == 0 {
			missing = append(missing, fmt.Sprintf("%s (not read)", p))
		} else {
			missing = append(missing, fmt.Sprintf("%s (read %d of %d lines; continue with Read offset=%d)", p, seen, total, next))
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("[infuse] Blocked: the project's instructions (%s) point to documents you have not yet read in full. Read each one to the end with Read before changing files or running commands — they are where the project says what has already been tried, what is forbidden and how to measure:\n- %s", strings.Join(r.Files, ", "), strings.Join(missing, "\n- "))
}

// instructionWriteCheck: the instruction files are not edited; the forbidden references are not written.
func (s *Session) instructionWriteCheck(rel, before string) string {
	r := loadProjectRules(s.Root)
	if isInstructionFile(rel) {
		return fmt.Sprintf("[infuse] Blocked: %s is the project's instructions. The agent follows the instructions; it does not change or delete them. If something in them needs to change, tell the user what and why.", rel)
	}
	if ref := underReference(r, rel); ref != "" {
		return fmt.Sprintf("[infuse] Blocked: %s is a read-only reference according to the project's instructions. Nothing is written in it.", ref)
	}
	return ""
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
	}
}

type runCount struct{ total, rounds, writesAt int }

type ranCommand struct {
	cmd         string
	ok          bool
	afterWrites int // changes made before it ran
}

// missingVerification lists the commands the instructions order after changes that did not run, with
// success, after this turn's last change ("" when none is missing or nothing changed).
func (s *Session) missingVerification() string {
	r := loadProjectRules(s.Root)
	s.mu.Lock()
	writes, ran := s.gate.writes, append([]ranCommand{}, s.gate.ran...)
	s.mu.Unlock()
	if writes == 0 || len(r.AfterChange) == 0 {
		return ""
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
		return ""
	}
	return fmt.Sprintf("[infuse] The turn cannot end: the project's instructions say to run these after changes, and they did not run successfully after this turn's last change: %s. Run exactly these commands (the project's metrics are its own, not tools of yours) and deal with whatever they fail.", strings.Join(missing, " · "))
}

// noteWrite and noteInvestigation feed the loop detector.
func (s *Session) noteWrite() { s.mu.Lock(); s.gate.writes++; s.mu.Unlock() }

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
func (s *Session) loopCheck(name, cat, path string, isAgentExplore bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := &s.gate
	if g.loopKey == "" {
		return ""
	}
	if isAgentExplore || investigates(s.Root, name, path) {
		g.studied++
		if g.studied >= 2 { // it went to the sources: the loop is over; counting starts again
			g.loopKey, g.studied, g.runs = "", 0, nil
		}
		return ""
	}
	if cat != "write" && cat != "exec" {
		return ""
	}
	docs := strings.Join(loadProjectRules(s.Root).Required, ", ")
	if docs == "" {
		return fmt.Sprintf("[infuse] Blocked: a measure-and-tweak loop — `%s` ran again after each tweak, 3 times or more in this turn, with no research into the cause. That treats effects, one at a time. Stop: read the project's documentation and the official documentation of what is involved (at least two readings), record in the plan (TodoWrite) the cause that explains all the symptoms together, and only then go back to changing things.", g.loopKey)
	}
	return fmt.Sprintf("[infuse] Blocked: a measure-and-tweak loop — `%s` ran again after each tweak, 3 times or more in this turn, with no research into the cause. That treats effects, one at a time. Stop: read the required documents (%s) and the official documentation of what is involved (at least two readings), record in the plan (TodoWrite) the cause that explains all the symptoms together, and only then go back to changing things.", g.loopKey, docs)
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
