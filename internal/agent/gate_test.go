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

func bashCall(id, cmd string) Block {
	in, _ := json.Marshal(map[string]string{"command": cmd})
	return Block{Type: "tool_use", ID: id, Name: "Bash", Input: in}
}

func TestResearchGateStopsTrialAndError(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("x\n"), 0o644)
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	run := func(b Block) (string, bool) { return s.runTool(context.Background(), &RuleSet{}, b) }

	run(bashCall("1", "echo 'Error: module not found' >&2; exit 1"))
	run(bashCall("2", "echo 'Error: module still not found' >&2; exit 1"))
	out, isErr := run(bashCall("3", "echo terceira tentativa"))
	if want := M("gate.research_local", "fails", 2, "errors", "Error: module not found\n- Error: module still not found"); !isErr || out != want.String() {
		t.Fatalf("third attempt without investigating was not blocked: %s", out)
	}
	// the refusal does not count as one more failed attempt
	if s.gate.fails != 2 {
		t.Fatalf("fails %d", s.gate.fails)
	}
	// reading the code opens the gate
	run(Block{Type: "tool_use", ID: "4", Name: "Read", Input: json.RawMessage(`{"path":"a.js"}`)})
	if out, isErr := run(bashCall("5", "echo depois de ler")); isErr {
		t.Fatalf("blocked after investigating: %s", out)
	}
	// failing again after reading: now only research outside the project opens it
	run(bashCall("6", "exit 1"))
	run(bashCall("7", "exit 1"))
	const web = "gate.research_web"
	if out, _ := run(bashCall("8", "echo de novo")); !msgSaid(s, "8", web) {
		t.Fatalf("second gate did not ask for web research: %s", out)
	}
	run(Block{Type: "tool_use", ID: "9", Name: "Read", Input: json.RawMessage(`{"path":"a.js"}`)})
	if out, isErr := run(bashCall("10", "echo so leitura local")); !isErr || !msgSaid(s, "10", web) {
		t.Fatalf("local reading opened a gate that asks for web research: %s", out)
	}
	if why := s.gateCheck("WebSearch", "net", json.RawMessage(`{"query":"module not found"}`)); !why.Empty() {
		t.Fatal(why)
	}
	if out, isErr := run(bashCall("11", "echo depois de pesquisar")); isErr {
		t.Fatalf("still blocked after web research: %s", out)
	}
	// a new turn starts clean
	s.gate.reset()
	if why := s.gateCheck("Bash", "exec", nil); !why.Empty() {
		t.Fatal(why)
	}
}

// msgSaid reports whether infuse recorded the message id for the call.
func msgSaid(s *Session, callID, id string) bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, m := range s.callMsgs[callID] {
		if m.ID == id {
			return true
		}
	}
	return false
}

func TestShortcutsInEditsAreCaught(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	s.gate.planned = true // the protocol's plan rule is tested apart
	test := filepath.Join(root, "soma.test.js")
	_ = os.WriteFile(test, []byte("test('soma', () => { expect(soma(2, 2)).toBe(4) })\n"), 0o644)
	s.markRead(test)
	// skipping the failing test: the user is asked even in bypass mode; refused here
	go func() {
		for i := 0; i < 300; i++ {
			s.Mu.Lock()
			req := s.pendingPerm
			s.Mu.Unlock()
			if req != nil {
				if !strings.Contains(req.Reason, ".skip") {
					t.Errorf("reason: %q", req.Reason)
				}
				s.ResolvePermission(req.ID, "deny")
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "1", Name: "Edit", Input: json.RawMessage(`{"path":"soma.test.js","old_string":"test('soma'","new_string":"test.skip('soma'"}`)})
	if want := M("shortcut.user_refused", "why", M("shortcut.weakens_tests", "items", []Msg{M("shortcut.skip_only")})); !isErr || out != want.String() {
		t.Fatal(out)
	}
	if b, _ := os.ReadFile(test); strings.Contains(string(b), "skip") {
		t.Fatal("the test was skipped although the user refused")
	}
	// swallowing an error in code: the edit goes through and the agent is told
	out, isErr = s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "2", Name: "Write", Input: json.RawMessage(`{"path":"soma.js","content":"try { carrega() } catch (e) {}\n"}`)})
	if want := M("shortcut.introduced", "items", []Msg{M("shortcut.empty_catch")}); isErr || !strings.Contains(out, "\n"+want.String()) {
		t.Fatal(out)
	}
	// a shortcut already in the file before the edit is not the agent's doing
	if ask, warn := detectShortcuts("x.js", "catch (e) {}", "catch (e) {}\nconst y = 1"); len(ask)+len(warn) != 0 {
		t.Fatal(ask, warn)
	}
}

func TestTurnWithChangesNeedsAReport(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Turn, s.TurnStarts = 1, []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "faça"}}}}
	if !s.missingReport().Empty() {
		t.Fatal("no changes, no report needed")
	}
	s.Checkpoints[1] = []CheckpointFile{{Path: "a.js", New: true}}
	if why := s.missingReport(); why.String() != M("gate.no_report", "count", 1).String() {
		t.Fatal(why)
	}
	s.Messages = append(s.Messages, Message{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "r", Name: "Report", Input: json.RawMessage(`{"summary":"x","claims":[]}`)}}})
	if !s.missingReport().Empty() {
		t.Fatal("report present")
	}
}

func TestDeepSeekHasAPriceSoBudgetsWork(t *testing.T) {
	testHome(t)
	s := LoadSettings()
	s.Pricing = map[string][2]float64{"gpt-5": {1.25, 10}} // an older settings file
	_ = SaveSettings(s)
	s = LoadSettings()
	if !s.HasPrice("deepseek-chat") || s.CostOf("deepseek-chat", Usage{Input: 1_000_000, Output: 1_000_000}) < 1.4 {
		t.Fatalf("deepseek-chat is free: %v", s.Pricing["deepseek-chat"])
	}
	if s.HasPrice("modelo-desconhecido") {
		t.Fatal("unknown model priced")
	}
}
