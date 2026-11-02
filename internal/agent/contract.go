package agent

// The Contract is the user-confirmed restatement of a request: the goal and the criteria that make
// it done. It is bound to the request text it was confirmed for, so a new request needs a new
// confirmation. While the setting is on, the first code write of a request is refused until the
// contract of that request is confirmed.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Contract is stored on the session, with the request text it belongs to.
type Contract struct {
	Goal     string   `json:"goal"`
	Criteria []string `json:"criteria"`
	Request  string   `json:"request"`
	Turn     int      `json:"turn"`
	At       int64    `json:"at"`
}

// contractGate returns why a write is refused before the request has a confirmed contract.
func (s *Session) contractGate() Msg {
	if s.settings.RequireContract != nil && !*s.settings.RequireContract {
		return Msg{}
	}
	s.Mu.Lock()
	c, turn := s.Contract, s.Turn
	alone := s.Unattended || s.Mission != nil && s.Mission.State == "running"
	s.Mu.Unlock()
	if alone { // the task or the mission's goal is the request the user wrote in advance
		return Msg{}
	}
	if c != nil && len(c.Criteria) > 0 && c.Turn == turn && c.Request == s.turnRequest() {
		return Msg{}
	}
	return M("contract.required")
}

type contractTool struct{}

func (contractTool) Category() string { return "ui" }
func (contractTool) Spec() ToolSpec {
	return ToolSpec{Name: "Contract", Description: "Before a piece of work with more than one step, turn the request into verifiable criteria (what must be true at the end, each one checkable by a command, a file or using the program) and ask the user to confirm them. Confirmed criteria go into the session memory with the highest priority. The first code write of a request is refused until its contract is confirmed.",
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
	request := tc.S.turnRequest()
	tc.S.Mu.Lock()
	tc.S.Contract = &Contract{Goal: in.Goal, Criteria: in.Criteria, Request: request, Turn: tc.S.Turn, At: time.Now().UnixMilli()}
	tc.S.Mu.Unlock()
	tc.S.save()
	tc.S.emit("memory", map[string]any{"action": "contract"})
	return tc.Say(M("contract.confirmed")), false
}
