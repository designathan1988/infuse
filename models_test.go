package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestContextWindowIsTheModels(t *testing.T) {
	for model, want := range map[string]int{"deepseek-flash": 1_000_000, "deepseek-chat": 1_000_000, "deepseek-v4-pro": 1_000_000, "claude-sonnet-5-5": 1_000_000, "claude-opus-5-5-20261001": 1_000_000, "llama-local": fallbackContext} {
		if got := contextWindow(model); got != want {
			t.Fatalf("%s: %d, want %d", model, got, want)
		}
	}
	testHome(t)
	s := newSession(t.TempDir())
	s.Model = "deepseek-chat"
	s.settings.ContextLimit = 0
	if s.contextLimit() != 1_000_000 {
		t.Fatal(s.contextLimit())
	}
	s.settings.ContextLimit = 300_000 // the user may ask for less, never for more than the model takes
	if s.contextLimit() != 300_000 {
		t.Fatal(s.contextLimit())
	}
	s.settings.ContextLimit = 5_000_000
	if s.contextLimit() != 1_000_000 {
		t.Fatal(s.contextLimit())
	}
	// the old fixed default migrates to the model's window
	_ = writeJSON(settingsFile(), map[string]any{"contextLimit": 200000})
	if st := LoadSettings(); st.ContextLimit != 0 {
		t.Fatalf("200000 kept: %d", st.ContextLimit)
	}
}

// The compaction that killed a session: the last exchange is a tool call and its results. After
// compacting, the call must still be there, right before its results.
func TestCompactionKeepsTheToolCallOfTheLastExchange(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Provider = "mock"
	call := Message{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "t1", Name: "Write", Input: json.RawMessage(`{"path":"a.ts","content":"x"}`)}}}
	result := Message{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "t1", Content: "Arquivo escrito"}}}
	s.Messages = make([]Message, 0, 16) // spare capacity: appending to a sub-slice would overwrite in place
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: "corrija a GPU"}}}, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "vou investigar"}}}, Message{Role: "user", Content: []Block{{Type: "text", Text: "continue"}}}, call, result)
	p, err := providerFor(s.settings, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.compact(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	n := len(s.Messages)
	if n != 4 || s.Messages[n-2].Role != "assistant" || s.Messages[n-2].Content[0].Type != "tool_use" || s.Messages[n-1].Content[0].ToolUseID != "t1" {
		b, _ := json.Marshal(s.Messages)
		t.Fatalf("the kept exchange was damaged: %s", b)
	}
	if got := repairTranscript(s.Messages); len(got) != 4 || got[3].Content[0].Type != "tool_result" {
		t.Fatal("a valid transcript was changed")
	}
}

func TestRepairTranscript(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "[Contexto compactado] resumo"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "Entendido"}}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "perdido", Content: "Arquivo escrito: a.ts"}}}, // its call was lost
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "c1", Name: "Read"}, {Type: "tool_use", ID: "c2", Name: "Read"}}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "c1", Content: "ok"}}}, // c2 never answered
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "c3", Name: "Bash"}}},       // interrupted at the end
	}
	got := repairTranscript(msgs)
	if got[2].Content[0].Type != "text" || !strings.Contains(got[2].Content[0].Text, "Arquivo escrito") {
		t.Fatalf("orphan result: %+v", got[2])
	}
	ids := map[string]bool{}
	for _, b := range got[4].Content {
		ids[b.ToolUseID] = true
	}
	if !ids["c1"] || !ids["c2"] {
		t.Fatalf("unanswered call: %+v", got[4])
	}
	if len(got) != 7 || got[6].Content[0].ToolUseID != "c3" || !got[6].Content[0].IsError {
		t.Fatalf("trailing call: %+v", got)
	}
}
