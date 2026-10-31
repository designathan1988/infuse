package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportClaimsCheckedAgainstTheTurn(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "old.js"), []byte("x\n"), 0o644)
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.Checkpoints[1] = []CheckpointFile{{Path: "a.js", New: true}}
	s.Messages = []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "faça"}}},
		{Role: "assistant", Content: []Block{
			{Type: "tool_use", ID: "1", Name: "Bash", Input: json.RawMessage(`{"command":"npm  test"}`)},
			{Type: "tool_use", ID: "2", Name: "Bash", Input: json.RawMessage(`{"command":"npm run build"}`)},
		}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "1", Content: "ok"}, {Type: "tool_result", ToolUseID: "2", Content: "exit 1\nerro TS2304", IsError: true}}},
	}
	s.inflight = []Block{} // a command whose result is still in flight counts too
	in := `{"summary":"feito","claims":[
		{"text":"testes passam","kind":"command","ref":"npm test"},
		{"text":"build ok","kind":"command","ref":"npm run build"},
		{"text":"lint ok","kind":"command","ref":"npm run lint"},
		{"text":"criei a.js","kind":"file","ref":"a.js"},
		{"text":"mexi em old.js","kind":"file","ref":"old.js"},
		{"text":"ficou mais rápido","kind":"other"}]}`
	out, isErr := reportTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(in))
	if isErr {
		t.Fatal(out)
	}
	for _, want := range []string{
		"[confirmado] testes passam",
		"[nao-confirmado] build ok — run in this turn, but it failed: exit 1",
		"[nao-confirmado] lint ok — this command was not run",
		"[confirmado] criei a.js — changed in this turn",
		"[parcial] mexi em old.js",
		"[sem-evidencia] ficou mais rápido",
		"2 claim(s) not confirmed",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}
