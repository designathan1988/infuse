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
	State      string        `json:"state"`            // running | aprovada | orcamento | impasse | parada
	Reason     string        `json:"reason,omitempty"` // why it ended, in the user's language (written from ReasonMsg)
	ReasonMsg  Msg           `json:"reasonMsg,omitempty"`
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

// missionNote is the structured content of a message infuse sends for a mission (Message.Kind
// "mission", on its text block's Data): the interface and the sentinel read the goal and the verdict
// here, never in the text.
type missionNote struct {
	Goal     string  `json:"goal"`
	Turn     int     `json:"turn,omitempty"` // a continuation: the mission's turn about to start
	MaxTurns int     `json:"maxTurns,omitempty"`
	Spent    float64 `json:"spent,omitempty"`
	Budget   float64 `json:"budget,omitempty"`
	Verdict  string  `json:"verdict,omitempty"` // the Done? verdict that sent the agent back
}

// missionNoteOf is the mission data of a message, when it is one of infuse's mission messages.
func missionNoteOf(m Message) (missionNote, bool) {
	var n missionNote
	if m.Kind != "mission" {
		return n, false
	}
	for _, b := range m.Content {
		if b.Type == "text" && len(b.Meta) > 0 && json.Unmarshal(b.Meta, &n) == nil {
			return n, true
		}
	}
	return n, false
}

// sendMission starts a turn with a mission message: English for the model, the user's language on
// screen, marked as a mission and carrying its data.
func (s *Session) sendMission(m Msg, note missionNote) error {
	text := m.String()
	if err := s.Send(text, nil, nil); err != nil {
		return err
	}
	raw, _ := json.Marshal(note)
	s.mu.Lock()
	if s.Turn >= 1 && s.Turn-1 < len(s.TurnStarts) {
		if i := s.TurnStarts[s.Turn-1]; i < len(s.Messages) && s.Messages[i].Role == "user" && len(s.Messages[i].Content) > 0 {
			c := append([]Block{}, s.Messages[i].Content...) // a copy: the turn already running holds the old one
			b := s.textBlockLocked(m)
			b.Meta = raw
			c[0] = b
			s.Messages[i].Content, s.Messages[i].Kind = c, "mission"
		}
	}
	if s.Turn == 1 && s.Title == truncate(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0], 60) {
		s.Title = truncate(note.Goal, 60) // the session is named by the goal, not by the instructions
	}
	s.mu.Unlock()
	s.emit("session", s.snapshot())
	return nil
}

// StartMission begins a mission on the session with the goal as the first message.
func (s *Session) StartMission(goal string, budget float64, maxTurns int) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return userError(s.uiLang(), M("mission.no_goal"))
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
		return userError(s.uiLang(), M("mission.busy"))
	}
	s.Mission = &Mission{Goal: goal, BudgetUSD: budget, MaxTurns: maxTurns, StartCost: s.Cost, StartTurn: s.Turn, State: "running", Started: time.Now().UnixMilli(), Steps: []MissionStep{}, lastCostAt: s.Cost, NoPrice: !s.settings.HasPrice(s.Model)}
	s.mu.Unlock()
	s.emit("mission", s.missionSnapshot())
	return s.sendMission(M("mission.brief", "goal", goal), missionNote{Goal: goal})
}

func (s *Session) missionSnapshot() *Mission {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.Mission.clone()
	if c != nil && !c.ReasonMsg.Empty() {
		c.Reason = c.ReasonMsg.Render(s.uiLang())
	}
	return c
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

// endMission ends the running mission with a reason given as text (already in the user's language).
func (s *Session) endMission(state, reason string) {
	s.endMissionWith(state, Msg{}, reason)
}

// endMissionMsg ends the running mission with a reason from the catalog.
func (s *Session) endMissionMsg(state string, reason Msg) {
	s.endMissionWith(state, reason, reason.Render(s.uiLang()))
}

func (s *Session) endMissionWith(state string, reason Msg, text string) {
	s.mu.Lock()
	if s.Mission != nil && s.Mission.State == "running" {
		s.Mission.State, s.Mission.Reason, s.Mission.ReasonMsg, s.Mission.Ended = state, text, reason, time.Now().UnixMilli()
	}
	s.mu.Unlock()
	s.save()
	s.emit("mission", s.missionSnapshot())
}

// StopMission is the user's stop.
func (s *Session) StopMission() {
	s.endMissionMsg("parada", M("mission.end_stopped"))
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

	usd := func(v float64) string { return fmt.Sprintf("%.2f", v) }
	switch {
	case step.Verdict == "aprovado":
		s.endMissionMsg("aprovada", M("mission.end_passed"))
		return
	case spent >= m.BudgetUSD:
		s.endMissionMsg("orcamento", M("mission.end_budget", "spent", usd(spent), "budget", usd(m.BudgetUSD)))
		return
	case turns >= m.MaxTurns:
		s.endMissionMsg("orcamento", M("mission.end_turns", "max", m.MaxTurns))
		return
	}
	var body Msg
	switch step.Verdict {
	case "sem-criterios":
		body = M("mission.no_criteria")
	case "erro":
		body = M("mission.cannot_verify", "error", step.Summary)
	default:
		body = M("mission.rejected", "failing", step.Summary)
	}
	auditor := Msg{} // renders as nothing when the auditor had nothing to say
	if step.Auditor != "" {
		auditor = M("mission.auditor", "note", step.Auditor)
	}
	next := M("mission.next", "turn", turns+1, "max", m.MaxTurns, "spent", usd(spent), "budget", usd(m.BudgetUSD), "body", body, "auditor", auditor)
	note := missionNote{Goal: m.Goal, Turn: turns + 1, MaxTurns: m.MaxTurns, Spent: spent, Budget: m.BudgetUSD, Verdict: step.Verdict}
	if err := s.sendMission(next, note); err != nil {
		s.endMissionMsg("parada", M("mission.end_cannot_continue", "error", errArg(err)))
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
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.fastModel(), Input: res.Usage.Input, Output: res.Usage.Output, Cost: s.settings.CostOf(s.fastModel(), res.Usage)})
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
		return tc.Say(M("impasse.no_mission")), false
	}
	s.endMission("impasse", im.Reason) // the agent's own words, shown as it wrote them
	return tc.Say(M("impasse.recorded")), false
}

func init() { registerTool(impasseTool{}) }
