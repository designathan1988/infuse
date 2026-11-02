package agent

import (
	"context"
	"encoding/json"
)

// AskUserQuestion.

// ---------------------------------------------------------------- AskUserQuestion

type askTool struct{}

func (askTool) Category() string { return "ui" }
func (askTool) Spec() ToolSpec {
	return ToolSpec{Name: "AskUserQuestion", Description: "Asks the user a question in the interface, with options. Use it when a decision belongs to the user and cannot be inferred.",
		InputSchema: schema(`{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}}},"required":["question"]}`)}
}

func (askTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	_ = json.Unmarshal(input, &in)
	if tc.S.mode() == "bypass" {
		return tc.Say(M("ask.bypass")), false
	}
	answer, ok := tc.S.askUser(ctx, in.Question, in.Options)
	if !ok {
		return tc.Say(M("ask.no_answer")), true
	}
	return tc.Say(M("ask.answer", "answer", answer)), false
}
