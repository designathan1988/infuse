package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSentinelAsksBeforeEditingTestsToMakeThemPass(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass" // even "accept everything" stops here
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "os testes estão falhando, faça passar"}}}}
	write := toolRegistry["Write"]

	if why := s.sentinel(write, json.RawMessage(`{"path":"src/soma.js","content":"x"}`)); why != "" {
		t.Fatal("code file flagged:", why)
	}
	for _, p := range []string{"src/soma.test.js", "tests/test_api.py", "pkg/x_test.go", "__tests__/a.js"} {
		if s.sentinel(write, json.RawMessage(`{"path":"`+p+`","content":"x"}`)) == "" {
			t.Fatal("test file not flagged:", p)
		}
	}

	// the user refuses: the write does not happen and the agent is told why
	go func() {
		for i := 0; i < 200; i++ {
			s.mu.Lock()
			req := s.pendingPerm
			s.mu.Unlock()
			if req != nil {
				if req.Reason == "" {
					t.Error("permission card without the sentinel's reason")
				}
				s.ResolvePermission(req.ID, "deny")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "1", Name: "Write", Input: json.RawMessage(`{"path":"src/soma.test.js","content":"test('x', () => {})"}`)})
	if !isErr || !strings.Contains(out, "teste") {
		t.Fatalf("%v %s", isErr, out)
	}

	// a request that is not about making tests pass: no sentinel
	s.Messages[0].Content[0].Text = "escreva testes para soma.js"
	if s.sentinel(write, json.RawMessage(`{"path":"src/soma.test.js"}`)) != "" {
		t.Fatal("writing new tests on request must not be flagged")
	}
}
