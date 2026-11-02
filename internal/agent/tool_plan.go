package agent

// EnterPlanMode and ExitPlanMode: the agent can narrow itself to reading and proposing; only the user,
// through the approval path, lets it out again.

import (
	"context"
	"encoding/json"
)

type enterPlanModeTool struct{}

func (enterPlanModeTool) Category() string { return "ui" }
func (enterPlanModeTool) Spec() ToolSpec {
	return ToolSpec{Name: "EnterPlanMode", Description: "Switches the session to plan mode: you may read and investigate, writing and execution are refused. Use it before a change large or uncertain enough to deserve the user's approval first. Leave with ExitPlanMode.",
		InputSchema: schema(`{"type":"object","properties":{}}`)}
}

func (enterPlanModeTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	s := tc.S
	s.Mu.Lock()
	current := s.mode()
	if current != "plan" {
		s.PlanReturn, s.Mode = current, "plan"
	}
	s.Mu.Unlock()
	if current == "plan" {
		return tc.Say(M("plan.already")), false
	}
	s.modeChanged()
	return tc.Say(M("plan.entered")), false
}

type exitPlanModeTool struct{}

func (exitPlanModeTool) Category() string { return "ui" }
func (exitPlanModeTool) Spec() ToolSpec {
	return ToolSpec{Name: "ExitPlanMode", Description: "Presents your plan to the user and asks to leave plan mode. If the user approves, the session returns to its previous mode and you carry out the plan; if not, you stay in plan mode and refine it.",
		InputSchema: schema(`{"type":"object","properties":{"plan":{"type":"string","description":"The plan to approve, in full"}},"required":["plan"]}`)}
}

func (t exitPlanModeTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Plan string }
	_ = json.Unmarshal(input, &in)
	s := tc.S
	if s.mode() != "plan" {
		return tc.Say(M("plan.not_in_plan")), true
	}
	if in.Plan != "" {
		s.Mu.Lock()
		s.Plan = in.Plan
		s.Mu.Unlock()
	}
	if !s.askPermissionWhy(ctx, t, input, "plan:exit", M("plan.exit_why", "plan", truncate(in.Plan, 4000))) {
		return tc.Say(M("plan.exit_refused")), false
	}
	s.Mu.Lock()
	back := firstNonEmpty(s.PlanReturn, "default")
	s.Mode, s.PlanReturn = back, ""
	s.Mu.Unlock()
	s.modeChanged()
	return tc.Say(M("plan.exit_approved", "mode", back)), false
}

// modeChanged keeps the new mode across a restart and tells the page.
func (s *Session) modeChanged() {
	s.save()
	s.emit("session", s.snapshot())
}
