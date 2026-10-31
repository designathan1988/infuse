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
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"       // even "accept everything" stops here
	s.gate.planned = true   // the protocol's plan rule is tested apart
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
	if !isErr || !strings.Contains(out, M("sentinel.test_edit").String()) {
		t.Fatalf("%v %s", isErr, out)
	}

	user := func(text string) Message { return Message{Role: "user", Content: []Block{{Type: "text", Text: text}}} }
	// infuse's mission messages carry the goal and the verdict as data: the sentinel reads those, not the text
	mission := func(m Msg, n missionNote) Message {
		raw, _ := json.Marshal(n)
		return Message{Role: "user", Kind: "mission", Content: []Block{{Type: "text", Text: m.String(), Meta: raw}}}
	}
	goal := "adicione uma função e um teste para ela em hello.test.js"
	// requests that are not about making failing tests pass: no sentinel
	for _, req := range []Message{user("escreva testes para soma.js"), mission(M("mission.brief", "goal", goal), missionNote{Goal: goal}), user("add a test for the parser")} {
		s.Messages[0] = req
		if why := s.sentinel(write, json.RawMessage(`{"path":"src/soma.test.js"}`)); why != "" {
			t.Fatalf("flagged %q", msgText(req))
		}
	}
	rejected := mission(M("mission.next", "turn", 2, "max", 5, "spent", "0.10", "budget", "2.00", "body", M("mission.rejected", "failing", "testes: exit 1"), "auditor", Msg{}), missionNote{Goal: goal, Turn: 2, MaxTurns: 5, Verdict: "reprovado"})
	for _, req := range []Message{user("os testes estão falhando, faça passar"), user("fix the failing tests"), user("corrija os testes"), user("make the tests pass"), rejected, user("o CI falhou")} {
		s.Messages[0] = req
		if s.sentinel(write, json.RawMessage(`{"path":"src/soma.test.js"}`)) == "" {
			t.Fatalf("not flagged %q", msgText(req))
		}
	}
}
