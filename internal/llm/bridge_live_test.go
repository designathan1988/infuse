package llm

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// TestBridgeTurnLive runs one real turn through each installed desktop CLI: a tool call comes back in
// the protocol, a tool result goes in, the final text comes out. INFUSE_LIVE=1 go test -run TestBridgeTurnLive
func TestBridgeTurnLive(t *testing.T) {
	if os.Getenv("INFUSE_LIVE") == "" {
		t.Skip("INFUSE_LIVE not set")
	}
	for _, tc := range []struct{ name, provider, model string }{
		{"claude", "claudedesk", "claude-sonnet-5-5"},
		{"codex", "codexdesk", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := NewProvider(tc.provider, ProviderConfig{})
			if err != nil {
				t.Fatal(err)
			}
			tools := []ToolSpec{{Name: "Read", Description: "Read one file", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}`)}}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
			defer cancel()
			r1, err := p.Stream(ctx, ModelRequest{Model: tc.model, System: "You are a test agent.", Messages: []Message{
				{Role: "user", Content: []Block{{Type: "text", Text: "Read the file notes.txt with the Read tool. Do not answer in prose; use the tool."}}},
			}, Tools: tools}, func(StreamEvent) {})
			if err != nil {
				t.Fatalf("turn 1: %v", err)
			}
			var call *Block
			for i := range r1.Content {
				if r1.Content[i].Type == "tool_use" {
					call = &r1.Content[i]
				}
			}
			if call == nil {
				t.Fatalf("turn 1 gave no tool call: %+v", r1.Content)
			}
			if call.Name != "Read" {
				t.Fatalf("tool name = %q, want Read", call.Name)
			}
			if r1.StopReason != "tool_use" {
				t.Fatalf("stop = %q, want tool_use", r1.StopReason)
			}
			t.Logf("turn 1 usage: %+v", r1.Usage)
			r2, err := p.Stream(ctx, ModelRequest{Model: tc.model, Messages: []Message{
				{Role: "user", Content: []Block{{Type: "text", Text: "Read the file notes.txt with the Read tool. Do not answer in prose; use the tool."}}},
				{Role: "assistant", Content: r1.Content},
				{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: call.ID, Content: "the quick brown fox"}}}}, Tools: tools}, func(StreamEvent) {})
			if err != nil {
				t.Fatalf("turn 2: %v", err)
			}
			text := ""
			for _, b := range r2.Content {
				if b.Type == "text" {
					text += b.Text
				}
			}
			if !strings.Contains(strings.ToLower(text), "fox") {
				t.Fatalf("final answer does not use the tool result: %q", text)
			}
			if r2.StopReason != "end_turn" {
				t.Fatalf("turn 2 stop = %q, want end_turn", r2.StopReason)
			}
		})
	}
}

// TestParseBridgeReply covers the JSON extraction the bridge depends on.
func TestParseBridgeReply(t *testing.T) {
	cases := []struct {
		name, in string
		wantErr  bool
	}{
		{"plain", `{"text":null,"tool_calls":[{"id":"c1","name":"Write","arguments":{"path":"a"}}]}`, false},
		{"fenced", "```json\n{\"text\":\"oi\",\"tool_calls\":[]}\n```", false},
		{"prose", `Here you go:\n{"text":"oi","tool_calls":[]}`, false},
		{"braces in string", `{"text":"use { and } freely","tool_calls":[]}`, false},
		{"garbage", "no json at all", true},
		{"truncated", `{"text":"never closed`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := parseBridgeReply(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", r)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestBridgeModelsList checks the static lists and that discovery finds the CLIs when installed.
func TestBridgeModelsList(t *testing.T) {
	if got := BridgeModels("claude"); len(got) != 3 || !strings.Contains(got[0], "sonnet") {
		t.Fatalf("claude models = %v", got)
	}
	if got := BridgeModels("codex"); len(got) != 1 || got[0] == "" {
		t.Fatalf("codex models = %v", got)
	}
	if p := FindClaudeCLI(); p == "" {
		t.Log("claude CLI not found on this machine (ok when not installed)")
	}
	if p := FindCodexCLI(); p == "" {
		t.Log("codex CLI not found on this machine (ok when not installed)")
	}
}
