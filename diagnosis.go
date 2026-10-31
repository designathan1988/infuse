package main

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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Evidence struct {
	File     string `json:"file"`
	Quote    string `json:"quote"`
	Supports string `json:"supports"` // what this shows about the problem
}

type Diagnosis struct {
	Turn      int        `json:"turn"`
	Problem   string     `json:"problem"`
	RootCause string     `json:"root_cause"`
	Approach  string     `json:"approach"`
	WhyCause  string     `json:"why_cause"` // why the approach treats the cause, not a symptom
	Evidence  []Evidence `json:"evidence"`
	At        int64      `json:"at"`
	Redo      string     `json:"redo,omitempty"` // why a new diagnosis was demanded
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
		return "invalid diagnosis: " + err.Error(), true
	}
	s := tc.S
	var cont struct {
		Continues bool `json:"continues"`
	}
	_ = json.Unmarshal(input, &cont)
	if cont.Continues { // the previous turn's diagnosis carries over, visibly, when nothing contradicted it
		s.mu.Lock()
		n, turn, redo := len(s.Diagnoses), s.Turn, s.gate.redoWhy
		var last Diagnosis
		if n > 0 {
			last = s.Diagnoses[n-1]
		}
		ok := n > 0 && last.Turn == turn-1 && redo == ""
		if ok {
			last.Turn, last.At = turn, time.Now().UnixMilli()
			last.Redo = "continues the diagnosis of the previous turn"
			s.Diagnoses = append(s.Diagnoses, last)
			s.gate.diagnosed = true
		}
		list := append([]Diagnosis{}, s.Diagnoses...)
		s.mu.Unlock()
		if !ok {
			return "[infuse] Diagnosis refused: continues=true only carries the diagnosis of the immediately previous turn, when no new diagnosis was demanded. Register a full Diagnosis.", true
		}
		s.emit("diagnoses", list)
		s.save()
		return "the previous diagnosis carries over to this turn (the user sees it marked as a continuation)", false
	}
	for _, f := range []struct{ name, v string }{{"problem", d.Problem}, {"root_cause", d.RootCause}, {"approach", d.Approach}, {"why_cause", d.WhyCause}} {
		if len(strings.Fields(f.v)) < 4 {
			return fmt.Sprintf("[infuse] Diagnosis refused: %s is empty or too short to mean anything.", f.name), true
		}
	}
	if len(d.Evidence) < 2 {
		return "[infuse] Diagnosis refused: at least 2 pieces of evidence (file and verbatim quote) are required.", true
	}
	var problems []string
	fromKnowledge := false
	knowledge := knowledgeInventory(s.Root)
	for i, e := range d.Evidence {
		rel := strings.TrimPrefix(filepath.ToSlash(e.File), "./")
		abs := filepath.Join(s.Root, filepath.FromSlash(rel))
		if !within(s.Root, abs) {
			problems = append(problems, fmt.Sprintf("evidence %d: %s is outside the project", i+1, e.File))
			continue
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			problems = append(problems, fmt.Sprintf("evidence %d: %s does not exist", i+1, e.File))
			continue
		}
		if q := normSpace(e.Quote); len(q) < 12 || !strings.Contains(normSpace(string(b)), q) {
			problems = append(problems, fmt.Sprintf("evidence %d: the quote is not in %s, word for word", i+1, rel))
			continue
		}
		if seen, total, next := s.readProgress(abs); seen < total {
			problems = append(problems, fmt.Sprintf("evidence %d: you read %d of %d lines of %s; read it to the end first (Read offset=%d)", i+1, seen, total, rel, next))
			continue
		}
		if contains(knowledge, rel) {
			fromKnowledge = true
		}
	}
	if len(problems) == 0 && len(knowledge) > 0 && !fromKnowledge {
		problems = append(problems, "no evidence comes from the project's own documents ("+strings.Join(firstN(knowledge, 8), ", ")+"…). What the project already knows about the problem comes first: read the relevant documents to the end and cite them")
	}
	s.mu.Lock()
	redo := s.gate.redoWhy
	prev := append([]Diagnosis{}, s.Diagnoses...)
	s.mu.Unlock()
	if len(problems) == 0 && redo != "" {
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
		if !fresh {
			problems = append(problems, "a new diagnosis was demanded ("+redo+") and it must bring evidence not cited before: the previous reading did not find the cause")
		}
		if len(prev) > 0 && normSpace(prev[len(prev)-1].RootCause) == normSpace(d.RootCause) {
			problems = append(problems, "the root cause is word for word the previous one, which the facts contradicted ("+redo+")")
		}
	}
	if len(problems) > 0 {
		return "[infuse] Diagnosis refused:\n- " + strings.Join(problems, "\n- "), true
	}
	s.mu.Lock()
	d.Turn, d.At, d.Redo = s.Turn, time.Now().UnixMilli(), redo
	s.Diagnoses = append(s.Diagnoses, d)
	s.gate.diagnosed, s.gate.redoWhy = true, ""
	list := append([]Diagnosis{}, s.Diagnoses...)
	s.mu.Unlock()
	s.emit("diagnoses", list)
	s.save()
	return fmt.Sprintf("diagnosis registered with %d verified pieces of evidence; the user sees it beside the plan. Now register the plan (TodoWrite) that follows from it.", len(d.Evidence)), false
}

func firstN(xs []string, n int) []string {
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

var reReadOnlyCmd = regexp.MustCompile(`^\s*(?:cd\s+\S+\s*&&\s*)?(?:git\s+(?:log|status|diff|show|blame|ls-files|grep|add|commit|push|stash|branch|switch|tag|fetch)|ls|dir|cat|head|tail|grep|rg|find|wc|tree|type|stat|file|which|where|pwd|echo)\b`)

// isReadOnlyCommand reports whether a shell command only reads (allowed before a diagnosis).
func isReadOnlyCommand(cmd string) bool {
	if helpCommand(cmd) {
		return true
	}
	for _, part := range regexp.MustCompile(`&&|\|\||;|\|`).Split(cmd, -1) {
		p := strings.TrimSpace(part)
		if p == "" || strings.HasPrefix(p, "cd ") {
			continue
		}
		if !reReadOnlyCmd.MatchString(p) || strings.Contains(p, ">") {
			return false
		}
	}
	return true
}

// diagnosisGate refuses changes and non-reading commands until the current request has a diagnosis.
func (s *Session) diagnosisGate(name, cat string, input json.RawMessage) string {
	if s.settings.RequireDiagnosis != nil && !*s.settings.RequireDiagnosis {
		return ""
	}
	if cat != "write" && cat != "exec" {
		return ""
	}
	if name == "Bash" && isReadOnlyCommand(bashCommand(input)) {
		return ""
	}
	s.mu.Lock()
	ok, redo := s.gate.diagnosed, s.gate.redoWhy
	s.mu.Unlock()
	if ok {
		return ""
	}
	if redo != "" {
		return "[infuse] Blocked: a new diagnosis is required (" + redo + "). The current approach produced that sign; continuing it treats effects. Read further — the project's documents first, to the end — and register a new Diagnosis with evidence you did not cite before."
	}
	return "[infuse] Blocked: no diagnosis for this request yet. Before changing files or running anything that is not a read, find out what the project already knows: read its instructions and the documents about this problem to the end, read the code involved, and register a Diagnosis (problem, root cause that explains all symptoms, approach, why it treats the cause, verbatim evidence)."
}

// demandRediagnosis marks the approach as contradicted by facts: changes wait for a new diagnosis.
func (s *Session) demandRediagnosis(why string) {
	s.mu.Lock()
	first := s.gate.redoWhy == ""
	s.gate.diagnosed, s.gate.redoWhy = false, why
	s.mu.Unlock()
	if first {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "novo diagnóstico exigido: " + why})
		s.emit("gate", map[string]any{"message": "[infuse] New diagnosis required: " + why + "."})
	}
}

func init() { registerTool(diagnosisTool{}) }
