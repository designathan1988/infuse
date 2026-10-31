package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// waitIdle waits for the turn to end: an answer after the last user message and the session idle.
func waitIdle(t *testing.T, s *Session) {
	t.Helper()
	for i := 0; i < 500; i++ {
		s.mu.Lock()
		st, n := s.State, len(s.Messages)
		last := ""
		if n > 0 {
			last = s.Messages[n-1].Role
		}
		s.mu.Unlock()
		if (st == "idle" || st == "error") && last == "assistant" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("session did not finish")
}

func TestHooksRunOnlyWhenApprovedAndCanBlock(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	block := `grep -q 'proibido' && { echo 'comando proibido pelo projeto' >&2; exit 2; } || exit 0`
	// written into the project (as the agent could): not approved, does nothing
	_ = addHook(root, "project", "PreToolUse", "Bash", block)
	in := Block{Type: "tool_use", ID: "1", Name: "Bash", Input: json.RawMessage(`{"command":"echo proibido"}`)}
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, in); isErr || strings.Contains(out, "hook") {
		t.Fatalf("an unapproved hook ran: %s", out)
	}
	h := LoadHooks(root)
	if len(h) != 1 || h[0].Approved {
		t.Fatalf("%+v", h)
	}
	_ = approveHook(h[0].Key)
	out, isErr := s.runTool(context.Background(), &RuleSet{}, in)
	if !isErr || !strings.Contains(out, "comando proibido pelo projeto") {
		t.Fatalf("not blocked: %s", out)
	}
	// a tool name the matcher does not cover is not affected
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "2", Name: "LS", Input: json.RawMessage(`{"path":"."}`)}); isErr {
		t.Fatal(out)
	}
	// removal
	if err := removeHook(root, h[0].Key); err != nil || len(LoadHooks(root)) != 0 {
		t.Fatal("remove", err)
	}
}

func TestStopHookSendsTheAgentBackOnce(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Provider = "mock"
	flag := filepath.ToSlash(filepath.Join(root, "ja-rodou"))
	cmd := `test -f '` + flag + `' && exit 0; touch '` + flag + `'; echo 'rode os testes antes de terminar' >&2; exit 2`
	_ = addHook(root, "project", "Stop", "", cmd)
	_ = approveHook(LoadHooks(root)[0].Key)
	if err := s.Send("oi", nil, nil); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, s)
	found := false
	for _, m := range s.Messages {
		found = found || strings.Contains(msgText(m), "[hook Stop] rode os testes antes de terminar")
	}
	if !found {
		t.Fatalf("stop hook reason not given to the agent: %+v", s.Messages)
	}
	if n := len(s.Messages); n != 4 { // user, answer, hook, answer — and then it stops
		t.Fatalf("%d messages", n)
	}
}

func TestUserPromptHookAddsContext(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Provider = "mock"
	_ = addHook(root, "global", "UserPromptSubmit", "", "echo 'branch: main'")
	_ = approveHook(LoadHooks(root)[0].Key)
	_ = s.Send("oi", nil, nil)
	waitIdle(t, s)
	if !strings.Contains(msgText(s.Messages[0]), "branch: main") {
		t.Fatalf("%q", msgText(s.Messages[0]))
	}
}

func TestSkillsFoundAndLoaded(t *testing.T) {
	testHome(t)
	home := t.TempDir() // the user's own ~/.claude/skills are not part of this test
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	root := t.TempDir()
	dir := filepath.Join(root, ".claude", "skills", "release")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\r\nname: release\r\ndescription: \"Publica uma versão\"\r\n---\r\n\r\n1. Rode os testes.\r\n2. Crie a tag.\r\n"), 0o644)
	sk := LoadSkills(root)
	if len(sk) != 1 || sk[0].Name != "release" || sk[0].Description != "Publica uma versão" {
		t.Fatalf("%+v", sk)
	}
	s := newSession(root)
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), "- release: Publica uma versão") {
		t.Fatal("skill not listed to the agent")
	}
	out, isErr := skillTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"name":"release"}`))
	if isErr || !strings.Contains(out, "1. Rode os testes.") || strings.Contains(out, "description:") {
		t.Fatal(out)
	}
}
