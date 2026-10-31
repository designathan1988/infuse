package agent

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
	Status   string `json:"status"`                // confirmado | parcial | nao-confirmado | sem-evidencia (a code, never shown as it is)
	Evidence Msg    `json:"evidenceMsg,omitempty"` // what infuse found, as a message of the catalog
	Display  string `json:"display,omitempty"`     // Evidence in the user's language, for the screen
}

// ReportSource is something the agent consulted in the turn: a web search, a page, a pinned document.
type ReportSource struct {
	Kind string `json:"kind"` // search | page | doc
	Ref  string `json:"ref"`  // the query, the address, the document's name
}

type TurnReport struct {
	Turn        int            `json:"turn"`
	CallID      string         `json:"callId,omitempty"` // the Report call this check belongs to
	Summary     string         `json:"summary"`
	Claims      []ReportClaim  `json:"claims"`
	Next        string         `json:"next,omitempty"`
	Sources     []ReportSource `json:"sources,omitempty"` // searches and pages read in the turn (provenance)
	Unconfirmed int            `json:"unconfirmed"`       // claims execution did not confirm
}

// msg is how a source is listed for the model.
func (r ReportSource) msg() Msg {
	switch r.Kind {
	case "search":
		return M("report.src_search", "query", r.Ref)
	case "doc":
		return M("report.src_doc", "name", r.Ref)
	}
	return M("report.src_page", "url", r.Ref)
}

// turnSources lists the web searches, pages and pinned docs the agent consulted in the current turn.
func (s *Session) turnSources() []ReportSource {
	s.Mu.Lock()
	start := 0
	if s.Turn-1 < len(s.TurnStarts) && s.Turn > 0 {
		start = s.TurnStarts[s.Turn-1]
	}
	msgs := append([]Message{}, s.Messages[min(start, len(s.Messages)):]...)
	s.Mu.Unlock()
	var out []ReportSource
	seen := map[ReportSource]bool{}
	for _, m := range msgs {
		for _, b := range m.Content {
			if b.Type != "tool_use" {
				continue
			}
			var in struct{ URL, Query, Path, Action string }
			_ = json.Unmarshal(b.Input, &in)
			var src ReportSource
			switch {
			case b.Name == "WebSearch":
				src = ReportSource{"search", in.Query}
			case b.Name == "WebFetch":
				src = ReportSource{"page", in.URL}
			case b.Name == "Browser" && in.Action == "navigate":
				src = ReportSource{"page", in.URL}
			case b.Name == "Read" && strings.Contains(filepath.ToSlash(in.Path), ".infuse/docs/"):
				src = ReportSource{"doc", filepath.Base(in.Path)}
			}
			if src.Ref != "" && !seen[src] {
				seen[src] = true
				out = append(out, src)
			}
		}
	}
	return out
}

func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

type claimContext struct {
	msgs    []Message
	written map[string]bool
	root    string
}

func (s *Session) claimContext() claimContext {
	s.Mu.Lock()
	defer s.Mu.Unlock()
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
	return claimContext{msgs: msgs, written: written, root: s.workRoot()}
}

// verifyClaim compares one claim with the turn's tool calls and checkpoints.
func (s *Session) verifyClaim(c *ReportClaim) {
	ctx := s.claimContext()
	switch c.Kind {
	case "command":
		s.verifyCommandClaim(c, ctx)
	case "file":
		verifyFileClaim(c, ctx)
	case "read": // "I read X": the lines infuse showed the agent, not its word
		s.verifyReadClaim(c, ctx.root)
	case "url": // "this page says…": the page was fetched in this session
		if len(s.unfetchedCitations(c.Ref)) == 0 && strings.TrimSpace(c.Ref) != "" {
			c.Status, c.Evidence = "confirmado", M("report.ev_fetched")
		} else {
			c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_fetched")
		}
	default:
		c.Kind, c.Status, c.Evidence = "other", "sem-evidencia", M("report.ev_none")
	}
}

func (s *Session) verifyCommandClaim(c *ReportClaim, ctx claimContext) {
	ref := squash(c.Ref)
	if ref == "" {
		c.Status, c.Evidence = "sem-evidencia", M("report.ev_no_command")
		return
	}
	results := map[string]Block{}
	for _, m := range ctx.msgs {
		for _, b := range m.Content {
			if b.Type == "tool_result" {
				results[b.ToolUseID] = b
			}
		}
	}
	found, failed := false, ""
	for _, m := range ctx.msgs {
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
		c.Status, c.Evidence = "confirmado", M("report.ev_ran")
	case failed != "":
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_failed", "output", failed)
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_run")
	}
}

func verifyFileClaim(c *ReportClaim, ctx claimContext) {
	rel := filepath.ToSlash(strings.TrimPrefix(filepath.Clean(c.Ref), filepath.Clean(ctx.root)+string(filepath.Separator)))
	rel = strings.TrimPrefix(rel, "./")
	_, err := os.Stat(filepath.Join(ctx.root, filepath.FromSlash(rel)))
	switch {
	case ctx.written[rel] && err == nil:
		c.Status, c.Evidence = "confirmado", M("report.ev_changed")
	case ctx.written[rel]:
		c.Status, c.Evidence = "confirmado", M("report.ev_removed")
	case err == nil:
		c.Status, c.Evidence = "parcial", M("report.ev_unchanged")
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_missing")
	}
}

func (s *Session) verifyReadClaim(c *ReportClaim, root string) {
	abs := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(filepath.ToSlash(c.Ref), "./")))
	seen, total, _ := s.readProgress(abs)
	switch {
	case total > 0 && seen >= total:
		c.Status, c.Evidence = "confirmado", M("report.ev_read_full", "total", total)
	case seen > 0:
		c.Status, c.Evidence = "parcial", M("report.ev_read_partial", "seen", seen, "total", total)
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_read")
	}
}

// claimStatus is a claim's status code as a message.
func claimStatus(code string) Msg {
	switch code {
	case "confirmado":
		return M("report.st_confirmed")
	case "parcial":
		return M("report.st_partial")
	case "nao-confirmado":
		return M("report.st_unconfirmed")
	}
	return M("report.st_no_evidence")
}

// checkReport verifies every claim of a report against the turn and returns what the model reads: one
// message per claim, the sources, and the count of claims execution did not confirm.
func (s *Session) checkReport(r *TurnReport) []Msg {
	lang := s.uiLang()
	out := []Msg{M("report.head")}
	r.Unconfirmed = 0
	for i := range r.Claims {
		s.verifyClaim(&r.Claims[i])
		c := &r.Claims[i]
		c.Display = c.Evidence.Render(lang)
		out = append(out, M("report.claim", "status", claimStatus(c.Status), "text", c.Text, "evidence", c.Evidence))
		if c.Status == "nao-confirmado" {
			r.Unconfirmed++
		}
	}
	r.Sources = s.turnSources()
	if len(r.Sources) > 0 {
		out = append(out, M("report.sources_head"))
		for _, src := range r.Sources {
			out = append(out, M("report.source", "source", src.msg()))
		}
	}
	if r.Unconfirmed > 0 {
		out = append(out, M("report.unconfirmed", "count", r.Unconfirmed))
	}
	return out
}

// attachToCall records structured data on the tool call it belongs to (the tool_use block of the
// current reply), where the screen reads it. The message is copied, not changed in place: the loop that
// runs the tools still holds the reply.
func (s *Session) attachToCall(callID string, data any) {
	if callID == "" {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for i := len(s.Messages) - 1; i >= 0; i-- {
		for j, b := range s.Messages[i].Content {
			if b.Type == "tool_use" && b.ID == callID {
				c := append([]Block{}, s.Messages[i].Content...)
				c[j].Meta = raw
				s.Messages[i].Content = c
				return
			}
		}
	}
}

// ---------------------------------------------------------------- tools

type reportTool struct{}

func (reportTool) Category() string { return "ui" }
func (reportTool) Spec() ToolSpec {
	return ToolSpec{Name: "Report", Description: "When you finish a piece of work, report what was done as claims, each with the evidence it relies on: kind=command with ref = the command you ran (build, tests), kind=file with ref = the file changed, or kind=other. infuse checks each claim against what actually happened in the turn and shows the result to the user. Put the suggested next step in next, if there is one.",
		InputSchema: schema(`{"type":"object","properties":{"summary":{"type":"string"},"claims":{"type":"array","items":{"type":"object","properties":{"text":{"type":"string"},"kind":{"type":"string","enum":["command","file","read","url","other"],"description":"command: a command you ran (ref = the command); file: a file you changed (ref = path); read: a file you read (ref = path; infuse checks you were shown every line); url: a page you cite (ref = address; infuse checks you fetched it); other: anything else (no evidence)"},"ref":{"type":"string"}},"required":["text","kind"]}},"next":{"type":"string"}},"required":["summary","claims"]}`)}
}

func (reportTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var r TurnReport
	if err := json.Unmarshal(input, &r); err != nil {
		return err.Error(), true
	}
	s := tc.S
	r.Turn, r.CallID = s.Turn, tc.CallID
	var lines []string
	for _, m := range s.checkReport(&r) {
		lines = append(lines, tc.Say(m))
	}
	// the screen draws the check from these data (status codes, evidence, sources), never from the text
	s.attachToCall(tc.CallID, r)
	s.emit("report", r)
	return strings.Join(lines, "\n"), false
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
		return tc.Say(M("contract.no_criteria")), true
	}
	var list strings.Builder
	for i, c := range in.Criteria {
		fmt.Fprintf(&list, "%d. %s\n", i+1, c)
	}
	// the question is the user's alone: written in their language; the answer is the option they chose
	lang := tc.S.uiLang()
	confirm := M("contract.confirm").Render(lang)
	ans, ok := tc.S.askUser(ctx, M("contract.question", "goal", in.Goal, "criteria", strings.TrimRight(list.String(), "\n")).Render(lang), []string{confirm})
	if !ok {
		return tc.Say(M("ask.no_answer")), true
	}
	if strings.TrimSpace(ans) != confirm {
		return tc.Say(M("contract.changes", "answer", ans)), false
	}
	for _, c := range in.Criteria {
		it := MemoryItem{Type: "criterio", Text: c, Priority: 3, Origin: "usuario", Session: tc.S.ID}
		if normalizeMemory(&it) == nil {
			_ = tc.S.editMemory("session", func(items []MemoryItem) []MemoryItem { return append(items, it) })
		}
	}
	tc.S.emit("memory", map[string]any{"action": "contract"})
	return tc.Say(M("contract.confirmed")), false
}
