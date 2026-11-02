package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func boolPointer(v bool) *bool { return &v }

// Multi-step work needs a Contract bound to the request of this turn; single-step work and a
// disabled policy do not.
func TestMultiStepWorkNeedsAConfirmedContract(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.settings.RequireContract = boolPointer(true)
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "build the feature"}}}}
	s.TurnStarts = []int{0}
	s.Turn = 1
	s.gate.planSteps = 2
	if why := s.contractGate(); why.ID != "contract.required" {
		t.Fatalf("multi-step work without a contract: %q", why.Render("en"))
	}
	s.Contract = &Contract{Goal: "build the feature", Request: "build the feature"}
	if why := s.contractGate(); !why.Empty() {
		t.Fatalf("a confirmed contract for this request was refused: %q", why.Render("en"))
	}
	s.Contract = &Contract{Goal: "old task", Request: "something else"}
	if why := s.contractGate(); why.ID != "contract.required" {
		t.Fatal("a contract for another request satisfied this one")
	}
	s.gate.planSteps = 1
	s.Contract = nil
	if why := s.contractGate(); !why.Empty() {
		t.Fatal("single-step work must not require a contract")
	}
	// tasks left from an earlier request are not this request's steps
	s.gate.reset()
	s.Tasks = []TaskItem{{Text: "a", Status: "pending"}, {Text: "b", Status: "pending"}}
	if why := s.contractGate(); !why.Empty() {
		t.Fatal("an old two-task plan demanded a contract for a new request")
	}
	s.gate.planSteps = 2
	s.settings.RequireContract = boolPointer(false)
	if why := s.contractGate(); !why.Empty() {
		t.Fatal("a disabled contract policy must not block")
	}
}

// A command that cannot write (a read, a search, output to /dev/null) does not need the contract.
func TestContractGateOnlyAppliesToWritingCommands(t *testing.T) {
	for _, cmd := range []string{"ls -la", "cat src/a.js", "grep -rn x src", "echo hi > /dev/null", "git status"} {
		if commandMayWrite(cmd) {
			t.Errorf("%q is not a write", cmd)
		}
	}
	for _, cmd := range []string{"echo x > out.txt", "cat > src/a.js <<'EOF'\nx\nEOF", "cp a b", "mkdir out", "sed -i s/a/b/ f", "curl -o f https://x", "node -e \"require('fs').writeFileSync('f','x')\""} {
		if !commandMayWrite(cmd) {
			t.Errorf("%q is a write", cmd)
		}
	}
}

// Shortening the plan after it demanded a contract does not erase the requirement.
func TestContractCannotBeAvoidedByShorteningThePlan(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.settings.RequireContract = boolPointer(true)
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "build the feature"}}}}
	s.TurnStarts, s.Turn = []int{0}, 1
	tc := &ToolContext{S: s, Root: s.Root, Cwd: s.Root}
	if out, isErr := (todoTool{}).Run(context.Background(), tc, json.RawMessage(`{"tasks":[{"id":"1","text":"a","status":"in_progress","proof":"test a"},{"id":"2","text":"b","status":"pending","proof":"test b"}]}`)); isErr {
		t.Fatal(out)
	}
	if why := s.contractGate(); why.ID != "contract.required" {
		t.Fatalf("a two-step plan did not demand a contract: %q", why.Render("en"))
	}
	if out, isErr := (todoTool{}).Run(context.Background(), tc, json.RawMessage(`{"tasks":[{"id":"1","text":"everything","status":"in_progress","proof":"one test"}]}`)); isErr {
		t.Fatal(out)
	}
	if why := s.contractGate(); why.ID != "contract.required" {
		t.Fatal("shortening the plan erased the confirmed restatement")
	}
}

// Abandoning work is visible: a blocked task must say why.
func TestBlockedTaskNeedsAReason(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.gate.planSteps = 1
	tc := &ToolContext{S: s, Root: s.Root, Cwd: s.Root}
	out, isErr := (todoTool{}).Run(context.Background(), tc, json.RawMessage(`{"tasks":[{"id":"1","text":"only","status":"blocked"}]}`))
	if !isErr || !strings.Contains(out, "reason") {
		t.Fatalf("a blocked task without a reason was accepted: %q", out)
	}
}
