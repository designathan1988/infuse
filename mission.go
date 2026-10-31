package main

// Missions: the agent works across turns on its own until the "Pronto?" criteria pass, within a budget
// (dollars and turns) the user sets. After each turn infuse runs the criteria in a snapshot (not the
// agent's word) and, if they fail, sends the agent back with the reasons. The mission ends approved,
// out of budget, stopped by the user, or at an impasse the agent declares with the Impasse tool (what it
// tried, the evidence, what it needs). A fast-model auditor comments on each step; its note is advice
// for the next turn, never a verdict. Every step is kept as a timeline (replay).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type MissionStep struct {
	Turn    int     `json:"turn"`
	At      int64   `json:"at"`
	Cost    float64 `json:"cost"`    // spent in this step
	Verdict string  `json:"verdict"` // aprovado | reprovado | sem-criterios | erro | ""
	Summary string  `json:"summary"` // the verdict's summary or the failing checks
	Auditor string  `json:"auditor,omitempty"`
	Run     string  `json:"run,omitempty"` // evidence run id
}

type Mission struct {
	Goal       string        `json:"goal"`
	BudgetUSD  float64       `json:"budgetUSD"`
	MaxTurns   int           `json:"maxTurns"`
	StartCost  float64       `json:"startCost"`
	StartTurn  int           `json:"startTurn"`
	State      string        `json:"state"` // running | aprovada | orcamento | impasse | parada
	Reason     string        `json:"reason,omitempty"`
	Started    int64         `json:"started"`
	Ended      int64         `json:"ended,omitempty"`
	Steps      []MissionStep `json:"steps"`
	Impasse    *Impasse      `json:"impasse,omitempty"`
	NoPrice    bool          `json:"noPrice,omitempty"` // the model has no known price: only the turn limit applies
	lastCostAt float64
}

type Impasse struct {
	Reason   string   `json:"reason"`
	Tried    []string `json:"tried"`
	Evidence string   `json:"evidence"`
	Needs    string   `json:"needs"`
}

// The "[Missão]" tag and the "Objetivo: " label are markers the interface (ui/app.js) and sentinel.go
// look for; they stay as they are until those readers change with them.
const missionBrief = `[Missão] Mission: work on your own until the goal below is met. At the end of each turn infuse runs the Done? criteria on a copy of the project; if they fail, you receive the reasons and continue. If you conclude that it is not possible to finish (missing access, information or a decision from the user), call the Impasse tool with what you tried and the evidence: that ends the mission and is a valid result.

Objetivo: `

// StartMission begins a mission on the session with the goal as the first message.
func (s *Session) StartMission(goal string, budget float64, maxTurns int) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return fmt.Errorf("descreva o objetivo da missão")
	}
	if budget <= 0 {
		budget = 2
	}
	if maxTurns <= 0 {
		maxTurns = 10
	}
	s.mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.mu.Unlock()
		return fmt.Errorf("o agente está ocupado")
	}
	s.Mission = &Mission{Goal: goal, BudgetUSD: budget, MaxTurns: maxTurns, StartCost: s.Cost, StartTurn: s.Turn, State: "running", Started: time.Now().UnixMilli(), Steps: []MissionStep{}, lastCostAt: s.Cost, NoPrice: !s.settings.hasPrice(s.Model)}
	s.mu.Unlock()
	s.emit("mission", s.missionSnapshot())
	return s.Send(missionBrief+goal, nil, nil)
}

func (s *Session) missionSnapshot() *Mission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Mission.clone()
}

// clone copies a mission for the UI (the original keeps changing under the session's lock).
func (m *Mission) clone() *Mission {
	if m == nil {
		return nil
	}
	c := *m
	c.Steps = append([]MissionStep{}, m.Steps...)
	return &c
}

func (s *Session) endMission(state, reason string) {
	s.mu.Lock()
	if s.Mission != nil && s.Mission.State == "running" {
		s.Mission.State, s.Mission.Reason, s.Mission.Ended = state, reason, time.Now().UnixMilli()
	}
	s.mu.Unlock()
	s.save()
	s.emit("mission", s.missionSnapshot())
}

// StopMission is the user's stop.
func (s *Session) StopMission() {
	s.endMission("parada", "parada pelo usuário")
	s.Stop()
}

// missionStep runs after each finished turn of a running mission: verdict, budget, auditor, next turn.
func (s *Session) missionStep() {
	s.mu.Lock()
	m := s.Mission
	if m == nil || m.State != "running" {
		s.mu.Unlock()
		return
	}
	turn, cost := s.Turn, s.Cost
	stepCost := cost - m.lastCostAt
	m.lastCostAt = cost
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	step := MissionStep{Turn: turn, At: time.Now().UnixMilli(), Cost: stepCost}
	v, err := RunOracle(ctx, s.Root, s.workRoot(), s.ID)
	step.Verdict, step.Run = v.Verdict, v.Run
	if err != nil {
		step.Verdict, step.Summary = "erro", err.Error()
	} else {
		var failing []string
		for _, c := range v.Checks {
			if !c.Passed {
				failing = append(failing, fmt.Sprintf("%s: %s", c.ID, c.Detail))
			}
		}
		step.Summary = v.Summary
		if len(failing) > 0 {
			step.Summary = strings.Join(failing, "\n")
		}
	}
	step.Auditor = s.auditTurn(ctx)
	s.mu.Lock()
	m.Steps = append(m.Steps, step)
	spent, turns := cost-m.StartCost, turn-m.StartTurn
	s.mu.Unlock()
	s.emit("mission", s.missionSnapshot())

	switch {
	case step.Verdict == "aprovado":
		s.endMission("aprovada", "os critérios do Pronto? passaram")
		return
	case spent >= m.BudgetUSD:
		s.endMission("orcamento", fmt.Sprintf("orçamento atingido: US$ %.2f de US$ %.2f", spent, m.BudgetUSD))
		return
	case turns >= m.MaxTurns:
		s.endMission("orcamento", fmt.Sprintf("limite de %d turnos atingido", m.MaxTurns))
		return
	}
	var next strings.Builder
	fmt.Fprintf(&next, "[Missão — turn %d of %d · US$ %.2f of US$ %.2f] ", turns+1, m.MaxTurns, spent, m.BudgetUSD)
	switch step.Verdict {
	case "sem-criterios":
		next.WriteString("There are no executable criteria in Done?, so infuse has no way to confirm the end. If the goal is met, report it with Report and call Impasse explaining that criteria are missing to confirm it; otherwise, continue.")
	case "erro":
		fmt.Fprintf(&next, "The criteria could not run: %s\nFix what prevents the verification or call Impasse.", step.Summary)
	default:
		fmt.Fprintf(&next, "Done? rejected the current work. Criteria that failed:\n%s\nFix it and continue. Do not change the criteria or the tests to make them pass.", step.Summary)
	}
	if step.Auditor != "" {
		fmt.Fprintf(&next, "\n\nNote from the flow auditor (advice, not an order): %s", step.Auditor)
	}
	if err := s.Send(next.String(), nil, nil); err != nil {
		s.endMission("parada", "não foi possível continuar: "+err.Error())
	}
}

// auditTurn asks the fast model for one or two sentences on the last turn's course: loops, ignored
// errors, work outside the goal. Empty when nothing stands out or no model answers.
func (s *Session) auditTurn(ctx context.Context) string {
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return ""
	}
	s.mu.Lock()
	start := 0
	if s.Turn-1 < len(s.TurnStarts) && s.Turn > 0 {
		start = s.TurnStarts[s.Turn-1]
	}
	msgs := append([]Message{}, s.Messages[min(start, len(s.Messages)):]...)
	goal := ""
	if s.Mission != nil {
		goal = s.Mission.Goal
	}
	s.mu.Unlock()
	var b strings.Builder
	for _, m := range msgs {
		for _, c := range m.Content {
			switch c.Type {
			case "text":
				fmt.Fprintf(&b, "%s: %s\n", m.Role, truncate(c.Text, 400))
			case "tool_use":
				fmt.Fprintf(&b, "tool %s %s\n", c.Name, truncate(string(c.Input), 200))
			case "tool_result":
				status := "ok"
				if c.IsError {
					status = "ERROR"
				}
				fmt.Fprintf(&b, "  → %s: %s\n", status, truncate(firstLine(c.Content), 160))
			}
		}
	}
	if b.Len() == 0 {
		return ""
	}
	ask := fmt.Sprintf("Mission goal: %s\n\nLog of the last turn of a coding agent:\n%s", goal, truncate(b.String(), 12000)) +
		"\n\nIn at most two sentences, point out a problem in how the work is being conducted, if there is one: repeating the same attempt, ignored errors, work outside the goal, changes to tests or criteria, claims without verification. If the work is being conducted properly, answer only: -"
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: ask}}}}, MaxTokens: 200}, func(StreamEvent) {})
	if err != nil {
		return ""
	}
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.fastModel(), Input: res.Usage.Input, Output: res.Usage.Output, Cost: s.settings.costOf(s.fastModel(), res.Usage)})
	out := strings.TrimSpace(msgText(Message{Content: res.Content}))
	if out == "-" || strings.HasPrefix(out, "-") && len(out) < 3 {
		return ""
	}
	return truncate(out, 600)
}

// ---------------------------------------------------------------- the Impasse tool

type impasseTool struct{}

func (impasseTool) Category() string { return "ui" }
func (impasseTool) Spec() ToolSpec {
	return ToolSpec{Name: "Impasse", Description: "Ends the running mission by declaring that it cannot be completed now: the reason, what was tried, the evidence (commands and outputs) and what would be needed (access, information, a decision from the user). An honest impasse is a valid result; carrying on with no way forward is not.",
		InputSchema: schema(`{"type":"object","properties":{"reason":{"type":"string"},"tried":{"type":"array","items":{"type":"string"}},"evidence":{"type":"string"},"needs":{"type":"string"}},"required":["reason","tried","needs"]}`)}
}

func (impasseTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var im Impasse
	_ = json.Unmarshal(input, &im)
	s := tc.S
	s.mu.Lock()
	running := s.Mission != nil && s.Mission.State == "running"
	if running {
		s.Mission.Impasse = &im
	}
	s.mu.Unlock()
	if !running {
		return "there is no mission running; explain the impasse to the user in your answer", false
	}
	s.endMission("impasse", im.Reason)
	return "Mission ended at an impasse. The user sees the reason, what was tried and what is needed. End the turn with a short summary.", false
}

func init() { registerTool(impasseTool{}) }
