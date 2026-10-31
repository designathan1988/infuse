package agent

// Diagnosis before action. An agent asked to fix something tends to act on the request literally —
// measure, patch the largest symptom, measure again — without finding out what the project already
// knows about the problem. infuse does not let it start that way: before the first change or command
// that is not a read, the agent registers a Diagnosis — the problem, its root cause, the approach and
// why it addresses the cause, with evidence. Each piece of evidence is a file and a verbatim quote:
// code checks that the quote is in the file and that the agent read that file to the end. When the
// knowledge inventory of the project is not empty, at least one piece of evidence comes from it.
// Objective signs that the approach is wrong (the measure-and-tweak loop, a declared check regressing,
// the agent undoing its own change, the breaker) demand a new diagnosis with new evidence.

import (
	"context"
	"encoding/json"
	"infuse/internal/shellparse"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Evidence struct {
	File     string `json:"file"`
	Quote    string `json:"quote"`
	Supports string `json:"supports"` // what this shows about the problem
}

type Diagnosis struct {
	Turn       int        `json:"turn"`
	Problem    string     `json:"problem"`
	RootCause  string     `json:"root_cause"`
	Approach   string     `json:"approach"`
	WhyCause   string     `json:"why_cause"` // why the approach treats the cause, not a symptom
	Evidence   []Evidence `json:"evidence"`
	At         int64      `json:"at"`
	Redo       Msg        `json:"redo,omitempty"`       // why a new diagnosis was demanded
	RedoText   string     `json:"redo_text,omitempty"`  // that reason in the user's language, for the screen
	Continuing bool       `json:"continuing,omitempty"` // carried over from the previous turn
}

// diagnosisRule is the line of the system prompt that states the protocol.
const diagnosisRule = "Before changing files or running anything that is not a read, register a Diagnosis (tool) with verified evidence; for a request that only continues the previous turn's work, call Diagnosis with continues=true. infuse refuses changes and commands until then, and demands a new diagnosis when facts contradict the approach."

type diagnosisTool struct{}

func (diagnosisTool) Category() string { return "ui" }
func (diagnosisTool) Spec() ToolSpec {
	return ToolSpec{Name: "Diagnosis", Description: `Register your diagnosis before changing files or running anything that is not a read. infuse refuses changes and commands until a valid diagnosis exists for the current request.
- problem: what is wrong or what is asked, in concrete terms.
- root_cause: the cause that explains ALL the symptoms together — not the largest symptom.
- approach: what you will change, at the level of the cause.
- why_cause: why this approach removes the cause instead of treating one effect.
- evidence: at least 2 items {file, quote, supports}. quote is copied verbatim from the file (code checks it), and you must have read that file to the end. When the project has documents (instructions, docs, memory, decisions, audits), at least one item must come from them: what the project already knows about this problem comes first.
If a new diagnosis is demanded, it must bring evidence you did not cite before.`,
		InputSchema: schema(`{"type":"object","properties":{"problem":{"type":"string"},"root_cause":{"type":"string"},"approach":{"type":"string"},"why_cause":{"type":"string"},"evidence":{"type":"array","items":{"type":"object","properties":{"file":{"type":"string"},"quote":{"type":"string"},"supports":{"type":"string"}},"required":["file","quote","supports"]}},"continues":{"type":"boolean","description":"true only when this request continues the previous turn's work and its diagnosis still holds"}},"required":[]}`)}
}

func (diagnosisTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var d Diagnosis
	if err := json.Unmarshal(input, &d); err != nil {
		return tc.Say(M("diag.invalid", "error", err.Error())), true
	}
	var cont struct {
		Continues bool `json:"continues"`
	}
	_ = json.Unmarshal(input, &cont)
	if cont.Continues { // the previous turn's diagnosis carries over, visibly, when nothing contradicted it
		return continueDiagnosis(tc)
	}
	for _, f := range []struct{ name, v string }{{"problem", d.Problem}, {"root_cause", d.RootCause}, {"approach", d.Approach}, {"why_cause", d.WhyCause}} {
		if len(strings.Fields(f.v)) < 4 {
			return tc.Say(M("diag.field_short", "field", f.name)), true
		}
	}
	if len(d.Evidence) < 2 {
		return tc.Say(M("diag.need_two")), true
	}
	knowledge := knowledgeInventory(tc.S.Root)
	problems, fromKnowledge := evidenceProblems(tc.S, d, knowledge)
	if len(problems) == 0 && len(knowledge) > 0 && !fromKnowledge {
		problems = append(problems, M("diag.no_knowledge", "docs", strings.Join(firstN(knowledge, 8), ", ")))
	}
	if len(problems) == 0 {
		problems = staleDiagnosis(tc.S, d, true)
	}
	if len(problems) > 0 {
		return tc.Say(M("diag.refused", "items", problems)), true
	}
	return registerDiagnosis(tc, d), false
}

func continueDiagnosis(tc *ToolContext) (string, bool) {
	s := tc.S
	s.Mu.Lock()
	n, turn, redo := len(s.Diagnoses), s.Turn, s.gate.redoWhy
	var last Diagnosis
	if n > 0 {
		last = s.Diagnoses[n-1]
	}
	ok := n > 0 && last.Turn == turn-1 && redo.Empty()
	if ok {
		last.Turn, last.At, last.Continuing, last.Redo = turn, time.Now().UnixMilli(), true, Msg{}
		s.Diagnoses = append(s.Diagnoses, last)
		s.gate.diagnosed = true
	}
	list := append([]Diagnosis{}, s.Diagnoses...)
	s.Mu.Unlock()
	if !ok {
		return tc.Say(M("diag.continue_refused")), true
	}
	s.emit("diagnoses", list)
	s.save()
	return tc.Say(M("diag.continued")), false
}

func evidenceProblems(s *Session, d Diagnosis, knowledge []string) ([]Msg, bool) {
	var problems []Msg
	fromKnowledge := false
	for i, e := range d.Evidence {
		n := i + 1
		rel := strings.TrimPrefix(filepath.ToSlash(e.File), "./")
		abs := filepath.Join(s.Root, filepath.FromSlash(rel))
		if !within(s.Root, abs) {
			problems = append(problems, M("diag.ev_outside", "n", n, "file", e.File))
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			problems = append(problems, M("diag.ev_missing", "n", n, "file", e.File))
			continue
		}
		if q := normSpace(e.Quote); len(q) < 12 || !strings.Contains(normSpace(string(b)), q) {
			problems = append(problems, M("diag.ev_quote", "n", n, "file", rel))
			continue
		}
		if seen, total, next := s.readProgress(abs); seen < total {
			problems = append(problems, M("diag.ev_unread", "n", n, "seen", seen, "total", total, "file", rel, "next", next))
			continue
		}
		if contains(knowledge, rel) {
			fromKnowledge = true
		}
	}
	return problems, fromKnowledge
}

// staleDiagnosis refuses a redone diagnosis that repeats the old evidence or cause.
func staleDiagnosis(s *Session, d Diagnosis, clean bool) []Msg {
	s.Mu.Lock()
	redo := s.gate.redoWhy
	prev := append([]Diagnosis{}, s.Diagnoses...)
	s.Mu.Unlock()
	if !clean || redo.Empty() {
		return nil
	}
	cited := map[string]bool{}
	for _, p := range prev {
		for _, e := range p.Evidence {
			cited[normSpace(e.File)+"|"+normSpace(e.Quote)] = true
		}
	}
	fresh := false
	for _, e := range d.Evidence {
		if !cited[normSpace(e.File)+"|"+normSpace(e.Quote)] {
			fresh = true
		}
	}
	var problems []Msg
	if !fresh {
		problems = append(problems, M("diag.not_new", "why", redo))
	}
	if len(prev) > 0 && normSpace(prev[len(prev)-1].RootCause) == normSpace(d.RootCause) {
		problems = append(problems, M("diag.same_cause", "why", redo))
	}
	return problems
}

func registerDiagnosis(tc *ToolContext, d Diagnosis) string {
	s := tc.S
	s.Mu.Lock()
	redo := s.gate.redoWhy
	d.Turn, d.At, d.Redo = s.Turn, time.Now().UnixMilli(), redo
	if !redo.Empty() {
		d.RedoText = redo.Render(s.uiLang())
	}
	s.Diagnoses = append(s.Diagnoses, d)
	s.gate.diagnosed, s.gate.redoWhy = true, Msg{}
	list := append([]Diagnosis{}, s.Diagnoses...)
	s.Mu.Unlock()
	s.emit("diagnoses", list)
	s.save()
	return tc.Say(M("diag.registered", "count", len(d.Evidence)))
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

// isReadOnlyCommand reports whether a shell command only reads (allowed before a diagnosis).
func isReadOnlyCommand(cmd string) bool {
	return helpCommand(cmd) || shellparse.DiagnosisReadOnly(cmd)
}

// diagnosisGate refuses changes and non-reading commands until the current request has a diagnosis.
func (s *Session) diagnosisGate(name, cat string, input json.RawMessage) Msg {
	if s.settings.RequireDiagnosis != nil && !*s.settings.RequireDiagnosis {
		return Msg{}
	}
	if cat != "write" && cat != "exec" {
		return Msg{}
	}
	if name == "Bash" && isReadOnlyCommand(bashCommand(input)) {
		return Msg{}
	}
	s.Mu.Lock()
	ok, redo := s.gate.diagnosed, s.gate.redoWhy
	s.Mu.Unlock()
	if ok {
		return Msg{}
	}
	if !redo.Empty() {
		return M("gate.redo", "why", redo)
	}
	return M("gate.no_diagnosis")
}

// demandRediagnosis marks the approach as contradicted by facts: changes wait for a new diagnosis.
func (s *Session) demandRediagnosis(why Msg) {
	s.Mu.Lock()
	first := s.gate.redoWhy.Empty()
	s.gate.diagnosed, s.gate.redoWhy = false, why
	s.Mu.Unlock()
	if first {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "novo diagnóstico exigido: " + why.Render("pt-BR")})
		s.emitMsg("gate", M("diag.redo_notice", "why", why))
	}
}
