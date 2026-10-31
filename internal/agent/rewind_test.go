package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRewindVariants(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	_ = os.WriteFile(file, []byte("v0"), 0o644)
	s := newSession(root)
	turn := func(text, content string) {
		s.Mu.Lock()
		s.Turn++
		s.TurnStarts = append(s.TurnStarts, len(s.Messages))
		s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: text}}}, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "ok"}}})
		s.Mu.Unlock()
		s.CheckpointBeforeWrite(file)
		_ = os.WriteFile(file, []byte(content), 0o644)
	}
	turn("primeiro", "v1")
	turn("segundo", "v2")

	// code only: file back to before turn 2, conversation intact
	p, err := s.RewindTo(2, true, false)
	if err != nil || p != "segundo" {
		t.Fatal(p, err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v1" || s.Turn != 2 || len(s.Messages) != 4 {
		t.Fatalf("code-only: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
	// conversation only: messages cut, file kept
	_ = os.WriteFile(file, []byte("v2b"), 0o644)
	if _, err := s.RewindTo(2, false, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v2b" || s.Turn != 1 || len(s.Messages) != 2 {
		t.Fatalf("conversation-only: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
	// both
	if _, err := s.RewindTo(1, true, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v0" || s.Turn != 0 || len(s.Messages) != 0 {
		t.Fatalf("both: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
}
