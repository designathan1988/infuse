package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiEditAllOrNothing(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	p := filepath.Join(root, "a.js")
	_ = os.WriteFile(p, []byte("const a = 1;\nconst b = 2;\n"), 0o644)
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}
	tool := multiEditTool{}
	if out, isErr := tool.Run(context.Background(), tc, json.RawMessage(`{"path":"a.js","edits":[{"old_string":"1","new_string":"10"}]}`)); !isErr || !strings.Contains(out, "Read") {
		t.Fatal("edited without reading:", out)
	}
	s.markRead(p)
	out, isErr := tool.Run(context.Background(), tc, json.RawMessage(`{"path":"a.js","edits":[{"old_string":"a = 1","new_string":"a = 10"},{"old_string":"does-not-exist","new_string":"x"}]}`))
	if !isErr || !strings.Contains(out, "edit 2") {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(p); string(b) != "const a = 1;\nconst b = 2;\n" {
		t.Fatal("a failed MultiEdit wrote part of the edits")
	}
	if out, isErr := tool.Run(context.Background(), tc, json.RawMessage(`{"path":"a.js","edits":[{"old_string":"a = 1","new_string":"a = 10"},{"old_string":"b = 2","new_string":"b = a"}]}`)); isErr {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(p); string(b) != "const a = 10;\nconst b = a;\n" {
		t.Fatalf("%q", b)
	}
	if out, isErr := tool.Run(context.Background(), tc, json.RawMessage(`{"path":".infuse/oracle/x.md","edits":[{"old_string":"a","new_string":"b"}]}`)); !isErr {
		t.Fatal("MultiEdit reached the oracle:", out)
	}
}

func TestReadImageAttachesIt(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	_ = os.WriteFile(filepath.Join(root, "screen.png"), []byte("\x89PNG\r\n\x1a\nfake"), 0o644)
	out, isErr := readTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"path":"screen.png"}`))
	if isErr || !strings.Contains(out, "attached") || len(s.pendingImages) != 1 || s.pendingImages[0].MediaType != "image/png" {
		t.Fatalf("%v %s %+v", isErr, out, s.pendingImages)
	}
}

func TestReadsOfOneReplyRunTogether(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	for _, f := range []string{"a.txt", "b.txt"} {
		_ = os.WriteFile(filepath.Join(root, f), []byte(f), 0o644)
	}
	content := []Block{
		{Type: "tool_use", ID: "1", Name: "Read", Input: json.RawMessage(`{"path":"a.txt"}`)},
		{Type: "tool_use", ID: "2", Name: "Read", Input: json.RawMessage(`{"path":"b.txt"}`)},
		{Type: "tool_use", ID: "3", Name: "Write", Input: json.RawMessage(`{"path":"c.txt","content":"x"}`)},
	}
	got := s.runReadsInParallel(context.Background(), &RuleSet{}, content)
	if len(got) != 2 || !strings.Contains(got["1"].Content, "a.txt") || !strings.Contains(got["2"].Content, "b.txt") {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["3"]; ok {
		t.Fatal("a write ran in the parallel batch")
	}
}

func TestExploreSubagentsRunAsATeam(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Provider = "mock"
	content := []Block{
		{Type: "tool_use", ID: "a", Name: "Agent", Input: json.RawMessage(`{"kind":"explore","prompt":"hello 1"}`)},
		{Type: "tool_use", ID: "b", Name: "Agent", Input: json.RawMessage(`{"kind":"review","prompt":"hello 2"}`)},
		{Type: "tool_use", ID: "c", Name: "Agent", Input: json.RawMessage(`{"kind":"general","prompt":"hello 3"}`)},
	}
	got := s.runReadsInParallel(context.Background(), &RuleSet{}, content)
	if len(got) != 2 || got["a"].Content == "" || got["b"].Content == "" {
		t.Fatalf("%+v", got)
	}
	if _, ok := got["c"]; ok {
		t.Fatal("a general subagent (may write) ran in the parallel batch")
	}
}
