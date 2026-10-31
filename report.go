package main

// The agent's report and the turn contract. Report: at the end of a turn the agent states what it did as
// claims, each with the evidence it relies on (a command it ran, a file it changed); infuse checks every
// claim against what actually happened in the turn — the tool calls and their results, the checkpoints —
// and shows the outcome beside the agent's words. Contract: the agent turns the order into verifiable
// criteria and the user confirms them; confirmed criteria become session memory of the highest priority.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ReportClaim struct {
	Text     string `json:"text"`
	Kind     string `json:"kind"` // command | file | other
	Ref      string `json:"ref,omitempty"`
	Status   string `json:"status"` // confirmado | parcial | nao-confirmado | sem-evidencia
	Evidence string `json:"evidence"`
}

type TurnReport struct {
	Turn    int           `json:"turn"`
	Summary string        `json:"summary"`
	Claims  []ReportClaim `json:"claims"`
	Next    string        `json:"next,omitempty"`
	Sources []string      `json:"sources,omitempty"` // searches and pages read in the turn (provenance)
}

// turnSources lists the web searches, pages and pinned docs the agent consulted in the current turn.
func (s *Session) turnSources() []string {
	s.mu.Lock()
	start := 0
	if s.Turn-1 < len(s.TurnStarts) && s.Turn > 0 {
		start = s.TurnStarts[s.Turn-1]
	}
	msgs := append([]Message{}, s.Messages[min(start, len(s.Messages)):]...)
	s.mu.Unlock()
	var out []string
	seen := map[string]bool{}
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type != "tool_use" {
				continue
			}
			var in struct{ URL, Query, Path, Action string }
			_ = json.Unmarshal(b.Input, &in)
			src := ""
			switch {
			case b.Name == "WebSearch":
				src = fmt.Sprintf("search: %s", in.Query)
			case b.Name == "WebFetch":
				src = in.URL
			case b.Name == "Browser" && in.Action == "navigate":
				src = in.URL
			case b.Name == "Read" && strings.Contains(filepath.ToSlash(in.Path), ".infuse/docs/"):
				src = fmt.Sprintf("pinned documentation: %s", filepath.Base(in.Path))
			}
			if src != "" && !seen[src] {
				seen[src] = true
				out = append(out, src)
			}
		}
	}
	return out
}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

// verifyClaim compares one claim with the turn's tool calls and checkpoints.
func (s *Session) verifyClaim(c *ReportClaim) {
	s.mu.Lock()
	start := 0
	if s.Turn-1 < len(s.TurnStarts) && s.Turn > 0 {
		start = s.TurnStarts[s.Turn-1]
	}
	msgs := append([]Message{}, s.Messages[min(start, len(s.Messages)):]...)
	msgs = append(msgs, Message{Role: "user", Content: append([]Block{}, s.inflight...)})
	written := map[string]bool{}
	for _, f := range s.Checkpoints[s.Turn] {
		written[f.Path] = true
	}
	root := s.workRoot()
	s.mu.Unlock()

	switch c.Kind {
	case "command":
		ref := squash(c.Ref)
		if ref == "" {
			c.Status, c.Evidence = "sem-evidencia", "no command given"
			return
		}
		results := map[string]Block{}
		for _, m := range msgs {
			for _, b := range m.Content {
				if b.Type == "tool_result" {
					results[b.ToolUseID] = b
				}
			}
		}
		found, failed := false, ""
		for _, m := range msgs {
			for _, b := range m.Content {
				if b.Type != "tool_use" || b.Name != "Bash" {
					continue
				}
				var in struct{ Command string }
				_ = json.Unmarshal(b.Input, &in)
				if !strings.Contains(squash(in.Command), ref) {
					continue
				}
				r, ok := results[b.ID]
				if ok && !r.IsError {
					found = true
				} else if ok {
					failed = firstLine(r.Content)
				}
			}
		}
		switch {
		case found:
			c.Status, c.Evidence = "confirmado", "run in this turn without error"
		case failed != "":
			c.Status, c.Evidence = "nao-confirmado", fmt.Sprintf("run in this turn, but it failed: %s", failed)
		default:
			c.Status, c.Evidence = "nao-confirmado", "this command was not run in this turn"
		}
	case "file":
		rel := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(c.Ref), filepath.Clean(root)+string(filepath.Separator)))
		rel = strings.TrimPrefix(rel, "./")
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		switch {
		case written[rel] && err == nil:
			c.Status, c.Evidence = "confirmado", "changed in this turn"
		case written[rel]:
			c.Status, c.Evidence = "confirmado", "removed in this turn"
		case err == nil:
			c.Status, c.Evidence = "parcial", "the file exists, but it was not changed in this turn"
		default:
			c.Status, c.Evidence = "nao-confirmado", "the file does not exist"
		}
	default:
		c.Kind, c.Status, c.Evidence = "other", "sem-evidencia", "a claim with no verifiable evidence"
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

// ---------------------------------------------------------------- tools

type reportTool struct{}

func (reportTool) Category() string { return "ui" }
func (reportTool) Spec() ToolSpec {
	return ToolSpec{Name: "Report", Description: "When you finish a piece of work, report what was done as claims, each with the evidence it relies on: kind=command with ref = the command you ran (build, tests), kind=file with ref = the file changed, or kind=other. infuse checks each claim against what actually happened in the turn and shows the result to the user. Put the suggested next step in next, if there is one.",
		InputSchema: schema(`{"type":"object","properties":{"summary":{"type":"string"},"claims":{"type":"array","items":{"type":"object","properties":{"text":{"type":"string"},"kind":{"type":"string","enum":["command","file","other"]},"ref":{"type":"string"}},"required":["text","kind"]}},"next":{"type":"string"}},"required":["summary","claims"]}`)}
}

func (reportTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var r TurnReport
	if err := json.Unmarshal(input, &r); err != nil {
		return err.Error(), true
	}
	s := tc.S
	r.Turn = s.Turn
	var b strings.Builder
	bad := 0
	for i := range r.Claims {
		s.verifyClaim(&r.Claims[i])
		c := r.Claims[i]
		fmt.Fprintf(&b, "- [%s] %s — %s\n", c.Status, c.Text, c.Evidence)
		if c.Status == "nao-confirmado" {
			bad++
		}
	}
	r.Sources = s.turnSources()
	s.emit("report", r)
	// The claim status codes ([confirmado], [nao-confirmado], …) and the "Fontes consultadas neste turno:"
	// header are read back by the interface (ui/app.js reportCard); they stay as they are until it changes.
	out := "Report checked by infuse (status codes: confirmado = confirmed, parcial = partial, nao-confirmado = not confirmed, sem-evidencia = no evidence):\n" + b.String()
	if len(r.Sources) > 0 {
		out += "Fontes consultadas neste turno:\n- " + strings.Join(r.Sources, "\n- ") + "\n"
	}
	if bad > 0 {
		out += fmt.Sprintf("%d claim(s) not confirmed by execution: fix the work or the claim; the user sees this check.", bad)
	}
	return out, false
}

type contractTool struct{}

func (contractTool) Category() string { return "ui" }
func (contractTool) Spec() ToolSpec {
	return ToolSpec{Name: "Contract", Description: "Before a piece of work with more than one step, turn the request into verifiable criteria (what must be true at the end, each one checkable by a command, a file or using the program) and ask the user to confirm them. Confirmed criteria go into the session memory with the highest priority.",
		InputSchema: schema(`{"type":"object","properties":{"goal":{"type":"string"},"criteria":{"type":"array","items":{"type":"string"}}},"required":["goal","criteria"]}`)}
}

func (contractTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Goal     string   `json:"goal"`
		Criteria []string `json:"criteria"`
	}
	_ = json.Unmarshal(input, &in)
	if len(in.Criteria) == 0 {
		return "give at least one criterion", true
	}
	var q strings.Builder
	fmt.Fprintf(&q, "Objetivo: %s\n\nVou considerar pronto quando:\n", in.Goal)
	for i, c := range in.Criteria {
		fmt.Fprintf(&q, "%d. %s\n", i+1, c)
	}
	q.WriteString("\nConfirma? (ou escreva o que ajustar)")
	ans, ok := tc.S.askUser(ctx, q.String(), []string{"Confirmar"})
	if !ok {
		return "the user did not answer", true
	}
	if strings.TrimSpace(ans) != "Confirmar" {
		return fmt.Sprintf("The user asked for changes to the criteria: %s\nRewrite them and call Contract again.", ans), false
	}
	for _, c := range in.Criteria {
		it := MemoryItem{Type: "criterio", Text: c, Priority: 3, Origin: "usuario", Session: tc.S.ID}
		if normalizeMemory(&it) == nil {
			_ = tc.S.editMemory("session", func(items []MemoryItem) []MemoryItem { return append(items, it) })
		}
	}
	tc.S.emit("memory", map[string]any{"action": "contract"})
	return "Criteria confirmed by the user and stored in the session memory. Work until they are met and report with Report, with evidence for each one.", false
}

func init() {
	registerTool(contractTool{})
	registerTool(reportTool{})
}
