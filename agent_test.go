package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubagentToolsAreLimitedAndGuarded(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Provider, s.Mode = "mock", "bypass"

	// explore: only read tools; the mock tries to Write and gets refused by the kind
	out, err := s.RunSubagent(context.Background(), "explore", "x", "escreva no oráculo")
	if err != nil || !strings.Contains(out, "unavailable for this subagent") {
		t.Fatalf("%v %s", err, out)
	}
	// general: Write is allowed by the kind, but the guard still protects the oracle
	out, err = s.RunSubagent(context.Background(), "general", "x", "escreva no oráculo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".infuse", "oracle", "done.md")); err == nil {
		t.Fatalf("a subagent wrote the oracle: %s", out)
	}
	if !subagentAllows("general", "Write") || subagentAllows("general", "Agent") || subagentAllows("review", "Bash") {
		t.Fatal("tool sets")
	}
}

func TestReviewMaterialHasRequestPlanCriteriaAndDiff(t *testing.T) {
	testHome(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git")
	}
	root := gitRepo(t)
	_ = os.WriteFile(filepath.Join(root, "novo.js"), []byte("module.exports = 1;\n"), 0o644)
	s := newSession(root)
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "crie novo.js exportando 1"}}}}
	s.Tasks = []TaskItem{{ID: "1", Text: "criar novo.js", Status: "done", Proof: "node -e require('./novo.js')"}}
	m := s.reviewMaterial("")
	for _, want := range []string{"crie novo.js exportando 1", "criar novo.js", "proof: node -e", "novo.js (new file)", "module.exports = 1;"} {
		if !strings.Contains(m, want) {
			t.Fatalf("missing %q in\n%s", want, m)
		}
	}
}

func TestIndependentReviewIsRecordedForTheAgent(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Provider = "mock"
	out, err := s.IndependentReview(context.Background(), "")
	if err != nil || out == "" {
		t.Fatal(err, out)
	}
	n := len(s.Messages)
	if n < 2 {
		t.Fatalf("%+v", s.Messages)
	}
	// what the messages are is data (Kind); the model reads the heading and the findings in English
	review, ack := s.Messages[n-2], s.Messages[n-1]
	if review.Kind != "review" || ack.Kind != "review_ack" || len(review.Content) != 2 {
		t.Fatalf("%+v", s.Messages)
	}
	if review.Content[0].Text != M("review.header").String() || review.Content[1].Text != out || ack.Content[0].Text != M("review.ack").String() {
		t.Fatalf("%+v", s.Messages)
	}
	if s.uiLang() != "en" && review.Content[0].Display != M("review.header").Render(s.uiLang()) {
		t.Fatalf("the heading is not shown in the user's language: %+v", review.Content[0])
	}
}
