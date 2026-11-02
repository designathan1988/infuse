package agent

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
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type researchGate struct {
	fails       int      // failed commands since the last investigation
	errors      []string // their first lines, shown when the gate closes
	trips       int      // how many times the gate closed in this turn
	needWeb     bool     // local reading was not enough: only outside research opens it
	unconfirmed int      // claims of the turn's latest Report that execution did not confirm

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

	oracleKey     string // the tree the last automatic verdict was produced from
	oracleVerdict string // that verdict's result code

	// gatedPaths holds, per path, the content a write tool wrote in this turn after every gate and the
	// user's decision. The shell audit skips a path only while it still has exactly that content: a
	// later shell edit of the same file is judged like any other.
	gatedPaths map[string]string

	structureAsked bool            // the turn's structure audit already sent the agent back once
	shellSeen      map[string]bool // shell changes already counted as this turn's writes
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

// gateCheck runs before a tool: it returns why the call is refused (empty when it may run).
func (s *Session) gateCheck(name string, cat string, input json.RawMessage) Msg {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	g := &s.gate
	if inv, web := isResearch(name, input); inv {
		if web || !g.needWeb {
			g.fails, g.errors = 0, nil
			if web {
				g.needWeb = false
			}
		}
		return Msg{}
	}
	if (cat != "exec" && cat != "write") || g.fails < 2 {
		return Msg{}
	}
	g.trips++
	if g.trips >= 2 {
		g.needWeb = true
	}
	if g.needWeb {
		return M("gate.research_web", "fails", g.fails, "errors", strings.Join(g.errors, "\n- "))
	}
	return M("gate.research_local", "fails", g.fails, "errors", strings.Join(g.errors, "\n- "))
}

// gateRecord runs after a tool: failed commands count toward the gate. A call that infuse refused before
// it acted (one of blockMsgs recorded for it) is not a failed attempt.
func (s *Session) gateRecord(callID, name, cat string, out string, isErr bool) {
	if cat != "exec" {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if !isErr {
		s.gate.fails, s.gate.errors = 0, nil
		return
	}
	for _, m := range s.callMsgs[callID] {
		if blockMsgs[m.ID] {
			return
		}
		out = strings.Replace(out, m.String(), "", 1) // infuse's own notes are not the command's error
	}
	s.gate.fails++
	s.gate.errors = append(s.gate.errors, lastMeaningfulLine(out))
	if len(s.gate.errors) > 4 {
		s.gate.errors = s.gate.errors[len(s.gate.errors)-4:]
	}
}

// lastMeaningfulLine is the line of a failed command's output that most likely carries the error (the
// notes infuse recorded for the call are already taken out).
func lastMeaningfulLine(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" || strings.HasPrefix(l, "exit") {
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
	what     Msg
	testOnly bool // weakens tests: the user decides
}

var shortcuts = []shortcut{
	{regexp.MustCompile(`\b(it|test|describe)\.skip\(|\bx(it|describe|test)\(|\btest\.todo\(`), M("shortcut.skip_only"), true},
	{regexp.MustCompile(`\b(it|test|describe)\.only\(|\bf(it|describe)\(`), M("shortcut.focus"), true},
	{regexp.MustCompile(`@pytest\.mark\.(skip|xfail)|\bpytest\.skip\(|\bunittest\.skip|@unittest\.skip`), M("shortcut.pytest_skip"), true},
	{regexp.MustCompile(`\bt\.Skip(f|Now)?\(`), M("shortcut.t_skip"), true},
	{regexp.MustCompile(`@(Disabled|Ignore)\b`), M("shortcut.disabled"), true},
	{regexp.MustCompile(`expect\((true|1|'.*?'|".*?")\)\.(toBe|toEqual)\(\s*(true|1|'.*?'|".*?")\s*\)|assert\.(ok|equal)\(\s*true\s*\)|\bassert\s+True\b`), M("shortcut.assert_true"), true},
	{regexp.MustCompile(`@ts-(ignore|nocheck|expect-error)`), M("shortcut.ts_ignore"), false},
	{regexp.MustCompile(`eslint-disable|//\s*nolint|#\s*noqa|#\s*type:\s*ignore|#\s*pylint:\s*disable`), M("shortcut.checker_silenced"), false},
	{regexp.MustCompile(`catch\s*(\([^)]*\))?\s*\{\s*\}|\.catch\(\s*\(\s*\w*\s*\)\s*=>\s*(\{\s*\}|null|undefined)\s*\)`), M("shortcut.empty_catch"), false},
	{regexp.MustCompile(`except(\s+[\w.,() ]+)?:\s*(\n\s*)?pass\b`), M("shortcut.except_pass"), false},
	{regexp.MustCompile(`"test"\s*:\s*"[^"]*(\|\|\s*true|exit\s+0)[^"]*"`), M("shortcut.test_true"), true},
}

// suppressors silence the project's checker. A suppression that carries its reason on the same line
// is a decision; one without a reason hides a problem. The reason forms are code, not prose: an
// eslint `-- reason`, specific linter/code names, a bracketed type-ignore code, or a comment inside
// an empty catch.
var suppressors = []*regexp.Regexp{
	regexp.MustCompile(`@ts-(?:ignore|nocheck|expect-error)`),
	regexp.MustCompile(`eslint-disable|//\s*nolint|#\s*noqa|#\s*type:\s*ignore|#\s*pylint:\s*disable`),
	regexp.MustCompile(`catch\s*(?:\([^)]*\))?\s*\{\s*(\/\*.*?\*\/)?\s*\}`),
	regexp.MustCompile(`except(?:\s+[\w.,() ]+)?:\s*(?:\n\s*)?pass\b`),
}

var (
	reJustifyWords = regexp.MustCompile(`--\s*\S+(?:\s+\S+)+|\/\*\s*\S+(?:\s+\S+)+\s*\*\/|#\s*\S+(?:\s+\S+){2,}|@ts-(?:ignore|expect-error)\s+\S+(?:\s+\S+)+`)
	reJustifyCode  = regexp.MustCompile(`(?://nolint|#\s*noqa|#\s*type:\s*ignore)\s*:\s*[\w-]+|#\s*noqa:\s*[A-Z]+\d*(?:,[A-Z]+\d*)+|#\s*type:\s*ignore\[[\w-]+`)
)

// unjustifiedSuppressions lists the lines in after (not in before) that silence a checker without
// saying why.
func unjustifiedSuppressions(before, after string) []string {
	was := map[string]int{}
	for _, l := range strings.Split(before, "\n") {
		was[l]++
	}
	var out []string
	for _, l := range strings.Split(after, "\n") {
		if was[l] > 0 {
			was[l]--
			continue
		}
		if suppressorWithoutReason(l) {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

func suppressorWithoutReason(line string) bool {
	matched := false
	for _, re := range suppressors {
		if re.MatchString(line) {
			matched = true
			break
		}
	}
	if !matched {
		return false
	}
	return !reJustifyWords.MatchString(line) && !reJustifyCode.MatchString(line)
}

// detectShortcuts lists the shortcuts that after has and before did not (newly introduced).
func detectShortcuts(path, before, after string) (ask []Msg, warn []Msg) {
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
	case "NotebookEdit":
		after = notebookEdited(before, input)
	default:
		return
	}
	return path, before, after, true
}

// missingReport returns the reminder when the turn changed files and ended without a Report (empty
// otherwise).
func (s *Session) missingReport() Msg {
	shell := len(s.ungatedChanges()) // a change made by a shell command owes the same account
	s.Mu.Lock()
	defer s.Mu.Unlock()
	changed := len(s.Checkpoints[s.Turn])
	if changed == 0 {
		changed = shell
	}
	if changed == 0 || s.Turn < 1 || s.Turn-1 >= len(s.TurnStarts) {
		return Msg{}
	}
	for _, m := range s.Messages[min(s.TurnStarts[s.Turn-1], len(s.Messages)):] {
		for _, b := range m.Content {
			if b.Type == "tool_use" && b.Name == "Report" {
				return Msg{}
			}
		}
	}
	return M("gate.no_report", "count", changed)
}

// unconfirmedReport refuses the end of a turn whose latest Report claims what execution did not
// confirm: the agent fixes the work or corrects the claim. "Not achieved" is a legitimate report; a
// false "achieved" is not.
func (s *Session) unconfirmedReport() Msg {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.gate.unconfirmed == 0 {
		return Msg{}
	}
	return M("gate.report_unconfirmed", "count", s.gate.unconfirmed)
}
