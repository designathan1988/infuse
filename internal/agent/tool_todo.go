package agent

import (
	"context"
	"encoding/json"
	"strings"
)

// TodoWrite: the visible plan.

// ---------------------------------------------------------------- TodoWrite

type todoTool struct{}

func (todoTool) Category() string { return "ui" }
func (todoTool) Spec() ToolSpec {
	return ToolSpec{Name: "TodoWrite", Description: "The plan for the request, visible to the user and checked off in real time. Required before changing files. Send the complete list on every update; each open task with a proof (how it will be proven). Status: pending, in_progress, done, blocked (with the reason in the Report). The turn only ends with all of them done or blocked.",
		InputSchema: schema(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","done","blocked"]},"proof":{"type":"string","description":"how this task will be proven: the command, the page or the file that shows it works"}},"required":["id","text","status"]}}},"required":["tasks"]}`)}
}

func (todoTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Tasks []TaskItem `json:"tasks"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return err.Error(), true
	}
	if len(in.Tasks) == 0 {
		return tc.Say(M("plan.empty")), true
	}
	var noProof, noReason []string
	open := false
	for _, t := range in.Tasks {
		if t.Status != "done" && t.Status != "blocked" {
			open = true
			if strings.TrimSpace(t.Proof) == "" {
				noProof = append(noProof, t.Text)
			}
		}
		if t.Status == "blocked" && strings.TrimSpace(t.Proof) == "" { // abandoning work is visible, never silent
			noReason = append(noReason, t.Text)
		}
	}
	if len(noProof) > 0 { // the protocol: every open task says how it will be proven
		return tc.Say(M("plan.no_proof", "tasks", strings.Join(noProof, "; "))), true
	}
	if len(noReason) > 0 {
		return tc.Say(M("plan.blocked_reason", "tasks", strings.Join(noReason, "; "))), true
	}
	tc.S.Mu.Lock()
	planned, existing := tc.S.gate.planned, len(tc.S.Tasks)
	tc.S.Mu.Unlock()
	if !open && !planned && existing == 0 {
		return tc.Say(M("plan.no_open")), true // a plan cannot be born already finished
	}
	for i := range in.Tasks {
		in.Tasks[i].Note = ""
	}
	tc.S.setTasks(in.Tasks)
	tc.S.Mu.Lock()
	tc.S.gate.planned = true
	tc.S.gate.planSteps = len(in.Tasks)
	if len(in.Tasks) >= 2 {
		tc.S.gate.contractNeeded = true // shortening the plan later does not erase the confirmed restatement
	}
	tc.S.Mu.Unlock()
	return tc.Say(M("plan.recorded", "count", len(in.Tasks))), false
}
