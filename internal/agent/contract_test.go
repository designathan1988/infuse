package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func contractSession(t *testing.T) *Session {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	s.gate.planned = true   // this test is about the contract gate
	s.gate.diagnosed = true // the gates before it are not under test here
	s.gate.planSteps = 2    // the rule binds multi-step work
	s.settings.RequireContract = boolPointer(true)
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: "add the widget"}}})
	s.TurnStarts = append(s.TurnStarts, len(s.Messages)-1)
	s.Turn = 1
	return s
}

func contractWriteCall(id, path string) Block {
	in, _ := json.Marshal(map[string]string{"path": path, "content": "package main\n"})
	return Block{Type: "tool_use", ID: id, Name: "Write", Input: in}
}

func TestFirstWriteNeedsTheConfirmedContract(t *testing.T) {
	s := contractSession(t)
	out, isErr := s.runTool(t.Context(), &RuleSet{}, contractWriteCall("w1", filepath.Join(s.Root, "main.go")))
	if !isErr || !msgSaid(s, "w1", "contract.required") {
		t.Fatalf("the write without a contract was not refused: %q isErr=%v", out, isErr)
	}

	// a contract of another request does not open the gate
	s.Mu.Lock()
	s.Contract = &Contract{Goal: "g", Criteria: []string{"works"}, Request: "an older request", Turn: 0}
	s.Mu.Unlock()
	if _, isErr := s.runTool(t.Context(), &RuleSet{}, contractWriteCall("w2", filepath.Join(s.Root, "main.go"))); !isErr {
		t.Fatal("a stale contract opened the gate")
	}

	// confirming the contract of the current request opens it
	s.Mu.Lock()
	s.Contract = &Contract{Goal: "g", Criteria: []string{"works"}, Request: "add the widget", Turn: 1}
	s.Mu.Unlock()
	if _, isErr := s.runTool(t.Context(), &RuleSet{}, contractWriteCall("w3", filepath.Join(s.Root, "main.go"))); isErr {
		t.Fatalf("write refused with a confirmed contract: %v", isErr)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "main.go")); err != nil {
		t.Fatalf("the write never landed: %v", err)
	}

	// prose is not code: it never needed the contract
	s.Mu.Lock()
	s.Contract = nil
	s.Mu.Unlock()
	if _, isErr := s.runTool(t.Context(), &RuleSet{}, contractWriteCall("w4", filepath.Join(s.Root, "docs", "notes.md"))); isErr {
		t.Fatal("a markdown write was refused for the contract")
	}
}

func TestContractGateFollowsTheSetting(t *testing.T) {
	s := contractSession(t)
	off := false
	s.settings.RequireContract = &off
	if _, isErr := s.runTool(t.Context(), &RuleSet{}, contractWriteCall("w1", filepath.Join(s.Root, "main.go"))); isErr && msgSaid(s, "w1", "contract.required") {
		t.Fatal("the gate ran while the setting was off")
	}
}

// noContract turns the contract gate off: the tests that use it are about the other gates.
func noContract(s *Session) {
	off := false
	s.settings.RequireContract = &off
}
