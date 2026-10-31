package main

import (
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
