package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sentinelSession(t *testing.T) (*Session, string) {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true
	s.gate.planned = true
	s.Mode = "bypass"
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "any request"}}}}
	return s, root
}

// An existing user test is refused deterministically when the change removes or alters its checks;
// no permission card is offered, in any mode.
func TestSentinelBlocksChangingAnExistingTest(t *testing.T) {
	s, root := sentinelSession(t)
	write := toolRegistry["Write"]
	for _, p := range []string{"src/sum.test.js", "tests/test_api.py", "pkg/x_test.go", "__tests__/a.js"} {
		if !s.sentinelMsg(write, json.RawMessage(`{"path":"`+p+`","content":"x"}`)).Empty() {
			t.Fatal("a new test file was flagged:", p)
		}
		abs := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte("test('x', () => { assert(true); });\n"), 0o644)
		if s.sentinelMsg(write, json.RawMessage(`{"path":"`+p+`","content":"x"}`)).ID != "sentinel.test_edit" {
			t.Fatal("an existing test file was not flagged:", p)
		}
	}
	if !s.sentinelMsg(write, json.RawMessage(`{"path":"src/sum.js","content":"x"}`)).Empty() {
		t.Fatal("a code file was flagged")
	}
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "1", Name: "Write", Input: json.RawMessage(`{"path":"src/sum.test.js","content":"test('x', () => {})"}`)})
	if !isErr || !strings.Contains(out, M("sentinel.test_edit_blocked", "file", "src/sum.test.js").String()) {
		t.Fatalf("%v %s", isErr, out)
	}
	if s.pendingPerm != nil {
		t.Fatal("the sentinel opened a permission card")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "src", "sum.test.js")); string(b) != "test('x', () => { assert(true); });\n" {
		t.Fatal("the refused write happened")
	}
}

// Adding new tests to an existing test file is legitimate work, not tampering.
func TestSentinelAllowsAddingToAnExistingTest(t *testing.T) {
	s, root := sentinelSession(t)
	abs := filepath.Join(root, "src", "sum.test.js")
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	_ = os.WriteFile(abs, []byte("test('existing', () => {});\n"), 0o644)
	write := toolRegistry["Write"]
	content := `{"path":"src/sum.test.js","content":"test('existing', () => {});\ntest('new', () => {});\n"}`
	if !s.sentinelMsg(write, json.RawMessage(content)).Empty() {
		t.Fatal("a pure addition to an existing test was flagged")
	}
	edit := toolRegistry["MultiEdit"]
	multi := `{"path":"src/sum.test.js","edits":[{"old_string":"test('existing'","new_string":"test('added', () => {});\ntest('existing'"}]}`
	if !s.sentinelMsg(edit, json.RawMessage(multi)).Empty() {
		t.Fatal("an additive MultiEdit was flagged")
	}
	skip := `{"path":"src/sum.test.js","edits":[{"old_string":"test('existing'","new_string":"test.skip('existing'"}]}`
	if s.sentinelMsg(edit, json.RawMessage(skip)).Empty() {
		t.Fatal("a change that disables an existing test was allowed")
	}
	// Correcting an expected value keeps the test's power and is legitimate work.
	_ = os.WriteFile(abs, []byte("test('sums', () => { assert(1 + 1 === 2); });\n"), 0o644)
	fix := `{"path":"src/sum.test.js","content":"test('sums', () => { assert(2 + 2 === 4); });\n"}`
	if !s.sentinelMsg(write, json.RawMessage(fix)).Empty() {
		t.Fatal("a corrected test expectation was blocked")
	}
}

// Bypass never opens a question or a permission card; the user delegated the decisions.
func TestBypassDoesNotAskTheUser(t *testing.T) {
	s, _ := sentinelSession(t)
	answer, ok := s.askUser(context.Background(), "choose", []string{"A", "B"})
	if !ok || answer != "A" || s.pendingAsk != nil {
		t.Fatalf("bypass asked the user: answer=%q ok=%v pending=%v", answer, ok, s.pendingAsk)
	}
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "ask", Name: "AskUserQuestion", Input: json.RawMessage(`{"question":"architecture?","options":["A","B"]}`)})
	if isErr || !strings.Contains(out, M("ask.bypass").String()) {
		t.Fatalf("bypass AskUserQuestion: %v %s", isErr, out)
	}
	contract := Block{Type: "tool_use", ID: "c", Name: "Contract", Input: json.RawMessage(`{"goal":"build the app","criteria":["the app runs"]}`)}
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, contract); isErr || s.Contract == nil {
		t.Fatalf("bypass contract asked or failed: %v %s", isErr, out)
	}
	if s.pendingAsk != nil || s.pendingPerm != nil {
		t.Fatal("bypass left a pending question or permission")
	}
}

// A test this session created itself belongs to the agent while it finishes the work.
func TestSentinelAllowsEditsToAgentCreatedTests(t *testing.T) {
	s, root := sentinelSession(t)
	write := toolRegistry["Write"]
	if out, isErr := s.runTool(context.Background(), &RuleSet{}, writeCall("w", "src/new.test.js", "test('x', () => {});\n")); isErr {
		t.Fatalf("creating a test was refused: %s", out)
	}
	if !s.agentFile("src/new.test.js") {
		t.Fatal("the created test was not tracked as the agent's")
	}
	abs := filepath.Join(root, "src", "new.test.js")
	if b, _ := os.ReadFile(abs); string(b) != "test('x', () => {});\n" {
		t.Fatalf("the test was not created: %q", b)
	}
	input := `{"path":"src/new.test.js","content":"test('x', () => {});\ntest('y', () => {});\n"}`
	if !s.sentinelMsg(write, json.RawMessage(input)).Empty() {
		t.Fatal("the agent's own test was still flagged")
	}
}
