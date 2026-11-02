package agent

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
	result := Message{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "t1", Content: "File written"}}}
	s.Messages = make([]Message, 0, 16) // spare capacity: appending to a sub-slice would overwrite in place
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: "fix the GPU"}}}, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "I will investigate"}}}, Message{Role: "user", Content: []Block{{Type: "text", Text: "continue"}}}, call, result)
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

// Compaction replaces the transcript; the current turn must still point at its own request, even when
// the request fell in the summarized part.
func TestCompactionKeepsTheCurrentTurnRequest(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Provider = "mock"
	call := Message{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "t1", Name: "Write", Input: json.RawMessage(`{"path":"a.ts","content":"x"}`)}}}
	result := Message{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "t1", Content: "File written"}}}
	s.Messages = []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "first request"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "one"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "second request"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "two"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "third request"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "working"}}},
		call, result,
	}
	s.Turn, s.TurnStarts = 3, []int{0, 2, 4}
	p, err := providerFor(s.settings, "mock")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.compact(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if len(s.TurnStarts) != 3 || s.TurnStarts[2] >= len(s.Messages) {
		t.Fatalf("turn table after compaction: %v of %d messages", s.TurnStarts, len(s.Messages))
	}
	if got := s.turnRequest(); got != "third request" {
		t.Fatalf("the current turn lost its request: %q (%v)", got, s.TurnStarts)
	}
}

// The fallback path (no model) keeps the same binding.
func TestFallbackCompactionKeepsTheCurrentTurnRequest(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	for i := 0; i < 12; i++ {
		role := "assistant"
		if i%2 == 0 {
			role = "user"
		}
		text := "message"
		if i == 4 {
			text = "current request"
		}
		s.Messages = append(s.Messages, Message{Role: role, Content: []Block{{Type: "text", Text: text}}})
	}
	s.Turn, s.TurnStarts = 3, []int{0, 2, 4}
	s.compactFallback()
	if len(s.TurnStarts) != 3 || s.TurnStarts[2] >= len(s.Messages) {
		t.Fatalf("turn table after fallback: %v of %d messages", s.TurnStarts, len(s.Messages))
	}
	if got := s.turnRequest(); got != "current request" {
		t.Fatalf("the fallback lost the current request: %q (%v)", got, s.TurnStarts)
	}
}

func TestRepairTranscript(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "[Compacted context] summary"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "Understood"}}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "lost", Content: "File written: a.ts"}}}, // its call was lost
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "c1", Name: "Read"}, {Type: "tool_use", ID: "c2", Name: "Read"}}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "c1", Content: "ok"}}}, // c2 never answered
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "c3", Name: "Bash"}}},       // interrupted at the end
	}
	got := repairTranscript(msgs)
	if got[2].Content[0].Type != "text" || !strings.Contains(got[2].Content[0].Text, "File written") {
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
