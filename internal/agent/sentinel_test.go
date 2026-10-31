package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// An existing test is the user's oracle: changing it always asks the user, whatever the request said
// (no words are read to decide); writing a new test is free.
func TestSentinelAsksBeforeChangingAnExistingTest(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"       // even "accept everything" stops here
	s.gate.planned = true   // the protocol's plan rule is tested apart
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "qualquer pedido"}}}}
	write := toolRegistry["Write"]
	for _, p := range []string{"src/soma.test.js", "tests/test_api.py", "pkg/x_test.go", "__tests__/a.js"} {
		if !s.sentinelMsg(write, json.RawMessage(`{"path":"`+p+`","content":"x"}`)).Empty() {
			t.Fatal("a new test file was flagged:", p)
		}
		abs := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte("existing test\n"), 0o644)
		if s.sentinelMsg(write, json.RawMessage(`{"path":"`+p+`","content":"x"}`)).ID != "sentinel.test_edit" {
			t.Fatal("an existing test file was not flagged:", p)
		}
	}
	if !s.sentinelMsg(write, json.RawMessage(`{"path":"src/soma.js","content":"x"}`)).Empty() {
		t.Fatal("a code file was flagged")
	}

	// the user refuses: the write does not happen and the agent is told why
	go func() {
		for i := 0; i < 200; i++ {
			s.Mu.Lock()
			req := s.pendingPerm
			s.Mu.Unlock()
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
	if b, _ := os.ReadFile(filepath.Join(root, "src", "soma.test.js")); string(b) != "existing test\n" {
		t.Fatal("the refused write happened")
	}
}
