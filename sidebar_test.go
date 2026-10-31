package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchSessionsLooksInsideConversations(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	RememberProject(root)
	s := newSession(root)
	s.Title = "Projeto Node"
	s.Messages = []Message{{Role: "assistant", Content: []Block{{Type: "text", Text: "Escolhi CommonJS por ser o caminho mais direto no Node 22, sem build."}}}}
	s.save()
	hits := searchSessions("commonjs", 10)
	if len(hits) != 1 || hits[0].ID != s.ID || !strings.Contains(hits[0].Snippet, "CommonJS") {
		t.Fatalf("%+v", hits)
	}
	if len(searchSessions("inexistente-xyz", 10)) != 0 {
		t.Fatal("false hit")
	}
	if got := snippetAround("ção ção ção", 4, 3); !strings.Contains(got, "ção") {
		t.Fatal(got)
	}
}

func TestConcurrentLoadsShareOneSession(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.save()
	id := s.ID
	sessionsMu.Lock()
	delete(sessions, id) // as after a restart: only on disk
	sessionsMu.Unlock()
	got := make(chan *Session, 50)
	for i := 0; i < 50; i++ {
		go func() { x, _ := loadSession(root, id); got <- x }()
	}
	first := <-got
	for i := 1; i < 50; i++ {
		if x := <-got; x != first {
			t.Fatal("two in-memory copies of the same session: events would go to one, the agent to the other")
		}
	}
}

func TestLocalChangesWithoutGit(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("um\ndois\ntres\n"), 0o644)
	s := newSession(root)
	s.Turn = 1
	s.checkpointBeforeWrite(filepath.Join(root, "a.js"))
	s.checkpointBeforeWrite(filepath.Join(root, "novo.js"))
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("um\nDOIS\ntres\nquatro\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "novo.js"), []byte("x\ny\n"), 0o644)
	ch := s.LocalChanges()
	files := ch["files"].([]fileChange)
	if len(files) != 2 || ch["added"] != 4 || ch["removed"] != 1 {
		t.Fatalf("%+v", ch)
	}
	if files[0].Path != "a.js" || files[0].Added != 2 || files[0].Removed != 1 || files[1].Status != "A" || files[1].Added != 2 {
		t.Fatalf("%+v", files)
	}
	d := s.LocalDiff("a.js")
	if !strings.Contains(d, "-dois") || !strings.Contains(d, "+DOIS") || !strings.Contains(d, "+quatro") || !strings.Contains(d, "@@ -1,3 +1,4 @@") {
		t.Fatal(d)
	}
	if err := s.LocalDiscard("a.js"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.js")); string(b) != "um\ndois\ntres\n" {
		t.Fatalf("%q", b)
	}
	if err := s.LocalDiscard("novo.js"); err != nil || exists(filepath.Join(root, "novo.js")) {
		t.Fatal("new file not removed")
	}
}

func TestForkAndPinMessages(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Title = "original"
	s.Turn, s.TurnStarts = 2, []int{0, 2}
	s.Messages = []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "um"}}},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "resposta um"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "dois"}}},
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "x", Name: "LS", Input: json.RawMessage(`{}`)}}},
	}
	n, err := s.Fork(3) // the dangling tool call is cut
	if err != nil || len(n.Messages) != 3 || n.Turn != 2 || n.Title != "↳ original" || n.ID == s.ID {
		t.Fatalf("%v %+v", err, n.SessionData)
	}
	if p := s.TogglePin(1); len(p) != 1 || p[0] != 1 {
		t.Fatal(p)
	}
	if p := s.TogglePin(1); len(p) != 0 {
		t.Fatal(p)
	}
}
