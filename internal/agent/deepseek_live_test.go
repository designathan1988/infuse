package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestDeepSeekThinkingWithToolsLive talks to the real DeepSeek API (key from the OS vault):
// thinking on with tools, the reasoning sent back on the next round (the API answers 400 when it is
// missing), and an older history without reasoning continuing in thinking mode.
// INFUSE_LIVE=1 go test -run TestDeepSeekThinkingWithToolsLive
func TestDeepSeekThinkingWithToolsLive(t *testing.T) {
	if os.Getenv("INFUSE_LIVE") == "" {
		t.Skip("INFUSE_LIVE not set")
	}
	p, err := providerFor(LoadSettings(), "deepseek")
	if err != nil {
		t.Fatal(err)
	}
	tools := []ToolSpec{{Name: "get_date", Description: "Devolve a data de hoje", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}}
	ctx := context.Background()
	old := []Message{ // an exchange from a session that ran without thinking
		{Role: "user", Content: []Block{{Type: "text", Text: "Olá"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "Olá! Como posso ajudar?"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "Que dia é hoje? Use a ferramenta get_date e depois responda numa frase."}}},
	}
	r1, err := p.Stream(ctx, ModelRequest{Model: "deepseek-chat", Messages: old, Tools: tools, MaxTokens: 4000, Thinking: 4000}, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("first round: %v", err)
	}
	var call *Block
	thought := ""
	for i, b := range r1.Content {
		if b.Type == "tool_use" {
			call = &r1.Content[i]
		}
		if b.Type == "thinking" {
			thought = b.Thinking
		}
	}
	t.Logf("round 1: stop=%s thinking=%d chars, call=%v", r1.StopReason, len(thought), call != nil)
	if thought == "" {
		t.Fatal("no reasoning with thinking on")
	}
	if call == nil {
		t.Fatal("the model did not call the tool")
	}
	msgs := append(old, Message{Role: "assistant", Content: r1.Content}, Message{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: call.ID, Content: "2026-10-31"}}})
	r2, err := p.Stream(ctx, ModelRequest{Model: "deepseek-chat", Messages: msgs, Tools: tools, MaxTokens: 4000, Thinking: 4000}, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("second round (reasoning sent back): %v", err)
	}
	text := msgText(Message{Content: r2.Content})
	t.Logf("round 2: %s", text)
	if !strings.Contains(text, "31") {
		t.Fatalf("answer without the tool's date: %q", text)
	}
	// thinking off (effort Low): no reasoning
	r3, err := p.Stream(ctx, ModelRequest{Model: "deepseek-chat", Messages: msgs, Tools: tools, MaxTokens: 2000, Thinking: 0}, func(StreamEvent) {})
	if err != nil {
		t.Fatalf("thinking off: %v", err)
	}
	for _, b := range r3.Content {
		if b.Type == "thinking" && strings.TrimSpace(b.Thinking) != "" {
			t.Fatal("reasoning with thinking off")
		}
	}
}
