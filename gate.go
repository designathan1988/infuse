package main

// Deterministic discipline for the agent, enforced by infuse (no model judges these):
//
// Research gate — two failed commands in a row block the next command or edit until the agent
// investigates: reads the code involved or consults a source. If failures go on after reading local
// files, the next gate accepts only research outside the project (WebSearch, WebFetch, the browser,
// pinned docs, a skill). Trial and error turns into understanding.
//
// Shortcut detector — an edit that newly introduces a known shortcut is caught: skipped or focused
// tests and assertions that cannot fail (ask the user first, in any mode); swallowed errors and
// silenced checkers (the edit goes through, the agent is told and the vice is recorded).
//
// Report requirement — a turn that changed files cannot end without a Report, whose claims infuse
// checks against what ran.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type researchGate struct {
	fails      int      // failed commands since the last investigation
	errors     []string // their first lines, shown when the gate closes
	trips      int      // how many times the gate closed in this turn
	needWeb    bool     // local reading was not enough: only outside research opens it
	reportAsks int      // times the agent was sent back for a Report in this turn

	planned        bool     // a plan (TodoWrite) was registered in this turn
	researched     []string // what research calls looked at in this turn (queries, addresses, doc files)
	obligationAsks int      // times the turn was refused an end
	reviewed       bool     // the mandatory review ran in this turn

	runs      map[string]*runCount // commands run in this turn, for the measure-and-tweak loop
	ran       []ranCommand         // every command of the turn with its outcome
	writes    int                  // changes made in this turn
	loopKey   string               // the command of a detected loop
	loopSince time.Time
	studied   int // investigations since the loop was detected

	diagnosed  bool                // the current request has a verified diagnosis
	redoWhy    Msg                 // why a new diagnosis is demanded (empty when none)
	fileHashes map[string][]string // contents each file had in this turn, to see the agent undo its own change
}

func (g *researchGate) reset() { *g = researchGate{} }

// isResearch reports whether a tool call counts as investigation, and whether it reaches outside the
// project (the web, the browser, pinned docs, a skill).
func isResearch(name string, input json.RawMessage) (investigates, web bool) {
	var in struct{ Path, Action, Kind string }
	_ = json.Unmarshal(input, &in)
	switch name {
	case "WebSearch", "WebFetch", "Skill":
		return true, true
	case "Browser":
		return true, in.Action == "navigate" || in.Action == "text"
	case "Agent":
		return in.Kind == "explore", in.Kind == "explore"
	case "Read":
		docs := strings.Contains(filepath.ToSlash(in.Path), ".infuse/docs/")
		return true, docs
	case "Grep", "Glob", "LS":
		return true, false
	}
	return strings.HasPrefix(name, "mcp__"), strings.HasPrefix(name, "mcp__")
}

// gateCheck runs before a tool: it returns why the call is refused, or "".
func (s *Session) gateCheck(name string, cat string, input json.RawMessage) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := &s.gate
	if inv, web := isResearch(name, input); inv {
		if web || !g.needWeb {
			g.fails, g.errors = 0, nil
			if web {
				g.needWeb = false
			}
		}
		return ""
	}
	if (cat != "exec" && cat != "write") || g.fails < 2 {
		return ""
	}
	g.trips++
	if g.trips >= 2 {
		g.needWeb = true
	}
	format := "[infuse] Blocked: %d attempts failed in a row. Before another attempt, read the code involved (Read, Grep) or consult a source (WebSearch, WebFetch, pinned documentation); then explain the cause, and only then fix it.\nRecent errors:\n- %s"
	if g.needWeb {
		format = "[infuse] Blocked: %d attempts failed in a row. Before another attempt, research outside the project: WebSearch with the error message, WebFetch on the official documentation for the version in use, the pinned documentation (.infuse/docs) or a skill — reading the local files was not enough; then explain the cause, and only then fix it.\nRecent errors:\n- %s"
	}
	return fmt.Sprintf(format, g.fails, strings.Join(g.errors, "\n- "))
}

// gateRecord runs after a tool: failed commands count toward the gate.
func (s *Session) gateRecord(name, cat string, out string, isErr bool) {
	if cat != "exec" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !isErr {
		s.gate.fails, s.gate.errors = 0, nil
		return
	}
	if strings.HasPrefix(out, "[infuse] Blocked") || strings.HasPrefix(out, "[infuse] Bloqueado") {
		return
	}
	s.gate.fails++
	s.gate.errors = append(s.gate.errors, lastMeaningfulLine(out))
	if len(s.gate.errors) > 4 {
		s.gate.errors = s.gate.errors[len(s.gate.errors)-4:]
	}
}

// lastMeaningfulLine is the line of a failed command's output that most likely carries the error.
func lastMeaningfulLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "[infuse") || strings.HasPrefix(l, "exit") {
			continue
		}
		if regexp.MustCompile(`(?i)error|erro|fail|exception|cannot|not found|undefined|expected`).MatchString(l) || i == 0 {
			return truncate(l, 200)
		}
	}
	if len(lines) > 0 {
		return truncate(strings.TrimSpace(lines[len(lines)-1]), 200)
	}
	return ""
}

// ---------------------------------------------------------------- shortcut detector

type shortcut struct {
	re       *regexp.Regexp
	what     string
	testOnly bool // weakens tests: the user decides
}

var shortcuts = []shortcut{
	{regexp.MustCompile(`\b(it|test|describe)\.(skip|only)\(|\bx(it|describe|test)\(|\btest\.todo\(`), "test disabled or isolated (.skip/.only)", true},
	{regexp.MustCompile(`@pytest\.mark\.(skip|xfail)|\bpytest\.skip\(|\bunittest\.skip|@unittest\.skip`), "test skipped (pytest skip/xfail)", true},
	{regexp.MustCompile(`\bt\.Skip(f|Now)?\(`), "test skipped (t.Skip)", true},
	{regexp.MustCompile(`@(Disabled|Ignore)\b`), "test disabled (@Disabled/@Ignore)", true},
	{regexp.MustCompile(`expect\((true|1|'.*?'|".*?")\)\.(toBe|toEqual)\(\s*(true|1|'.*?'|".*?")\s*\)|assert\.(ok|equal)\(\s*true\s*\)|\bassert\s+True\b`), "assertion that cannot fail", true},
	{regexp.MustCompile(`@ts-(ignore|nocheck|expect-error)`), "type checking silenced (@ts-ignore)", false},
	{regexp.MustCompile(`eslint-disable|//\s*nolint|#\s*noqa|#\s*type:\s*ignore|#\s*pylint:\s*disable`), "checker silenced", false},
	{regexp.MustCompile(`catch\s*(\([^)]*\))?\s*\{\s*\}|\.catch\(\s*\(\s*\w*\s*\)\s*=>\s*(\{\s*\}|null|undefined)\s*\)`), "swallowed error (empty catch)", false},
	{regexp.MustCompile(`except(\s+[\w.,() ]+)?:\s*(\n\s*)?pass\b`), "swallowed error (except: pass)", false},
	{regexp.MustCompile(`"test"\s*:\s*"[^"]*(\|\|\s*true|exit\s+0)[^"]*"`), "test script that never fails (|| true)", true},
}

// detectShortcuts lists the shortcuts that after has and before did not (newly introduced).
func detectShortcuts(path, before, after string) (ask []string, warn []string) {
	isTest := reTestFile.MatchString(filepath.ToSlash(path)) || strings.HasSuffix(filepath.ToSlash(path), "package.json")
	for _, sc := range shortcuts {
		if len(sc.re.FindAllStringIndex(after, -1)) <= len(sc.re.FindAllStringIndex(before, -1)) {
			continue
		}
		if sc.testOnly && isTest {
			ask = append(ask, sc.what)
		} else if !sc.testOnly {
			warn = append(warn, sc.what)
		}
	}
	return
}

// editedContent returns the file's text before and after a Write, Edit or MultiEdit (without writing).
func editedContent(root, cwd string, name string, input json.RawMessage) (path, before, after string, ok bool) {
	var in struct {
		Path       string `json:"path"`
		FilePath   string `json:"file_path"`
		Content    string `json:"content"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
		Edits      []struct {
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		} `json:"edits"`
	}
	if json.Unmarshal(input, &in) != nil {
		return
	}
	path = firstNonEmpty(in.Path, in.FilePath)
	if path == "" {
		return
	}
	abs := FromMsysPath(path)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	b, _ := os.ReadFile(abs)
	before = string(b)
	switch name {
	case "Write":
		after = in.Content
	case "Edit":
		after = strings.Replace(before, in.OldString, in.NewString, map[bool]int{true: -1, false: 1}[in.ReplaceAll])
	case "MultiEdit":
		after = before
		for _, e := range in.Edits {
			after = strings.Replace(after, e.OldString, e.NewString, map[bool]int{true: -1, false: 1}[e.ReplaceAll])
		}
	default:
		return
	}
	return path, before, after, true
}

// missingReport returns the reminder when the turn changed files and ended without a Report ("" otherwise).
func (s *Session) missingReport() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := len(s.Checkpoints[s.Turn])
	if changed == 0 || s.gate.reportAsks > 0 || s.Turn < 1 || s.Turn-1 >= len(s.TurnStarts) {
		return ""
	}
	for _, m := range s.Messages[min(s.TurnStarts[s.Turn-1], len(s.Messages)):] {
		for _, b := range m.Content {
			if b.Type == "tool_use" && b.Name == "Report" {
				return ""
			}
		}
	}
	return fmt.Sprintf("[infuse] This turn changed %d file(s) and ended without a Report. Before finishing, report with the Report tool: every claim with the evidence it rests on (the command you ran, the file you changed). infuse checks each one against what actually happened.", changed)
}
