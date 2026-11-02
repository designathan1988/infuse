package agent

import (
	"context"
	"encoding/json"
	"infuse/internal/workspace"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// turnOn opens a turn the way Send does, without starting the loop.
func turnOn(t *testing.T, s *Session, request string) {
	t.Helper()
	s.Turn++
	s.TurnStarts = append(s.TurnStarts, len(s.Messages))
	s.TurnRequest = request
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: request}}})
	s.gate.reset()
	if err := snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn); err != nil {
		t.Fatal(err)
	}
}

func lockSession(t *testing.T) *Session {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return newSession(root)
}

func changedPaths(s *Session) string {
	var out []string
	for _, ch := range s.ungatedChanges() {
		out = append(out, ch.Path)
	}
	return strings.Join(out, ",")
}

// A clean Write must not buy the shell a free pass on the same path.
func TestGatedPathIsJudgedAgainOnceItsContentChanges(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	p := filepath.Join(s.Root, "a.txt")
	if err := os.WriteFile(p, []byte("written by the tool\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.markGated("a.txt", p)
	if got := changedPaths(s); got != "" {
		t.Fatalf("the tool's own write was audited: %q", got)
	}
	if err := os.WriteFile(p, []byte("changed by sed afterwards\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := changedPaths(s); got != "a.txt" {
		t.Fatalf("the later shell edit escaped the audit: %q", got)
	}
}

func TestShellEditOfAnExistingTestIsRefused(t *testing.T) {
	s := lockSession(t)
	p := filepath.Join(s.Root, "a_test.go")
	if err := os.WriteFile(p, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	turnOn(t, s, "edit")
	if err := os.WriteFile(p, []byte("package a\n\n// changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, ch := range s.ungatedChanges() {
		if m := s.shellChangeViolation(ch); m.ID == "gate.shell_test_edit" {
			found = true
		}
	}
	if !found {
		t.Fatal("sed on a test passed every lock")
	}
}

// The contract binds a shell write of code exactly as it binds Write.
func TestShellCodeWriteNeedsTheContract(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	s.gate.planned = true
	if err := os.WriteFile(filepath.Join(s.Root, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := s.auditShellWrites(); m.ID != "contract.required" {
		t.Fatalf("code written by the shell skipped the contract: %+v", m)
	}
}

func TestShellChangesCountAsTheTurnsWrites(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	if err := os.WriteFile(filepath.Join(s.Root, "a.txt"), []byte("by the shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.noteShellChanges()
	s.noteShellChanges()
	if s.gate.writes != 1 {
		t.Fatalf("writes = %d, want 1", s.gate.writes)
	}
	if m := s.missingReport(); m.ID != "gate.no_report" {
		t.Fatalf("a shell change owes a Report: %+v", m)
	}
}

func TestUnconfirmedReportHoldsTheTurn(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	s.checkReport(&TurnReport{Claims: []ReportClaim{{Text: "ran", Kind: "command", Ref: "go test ./..."}}})
	if m := s.unconfirmedReport(); m.ID != "gate.report_unconfirmed" {
		t.Fatalf("a false claim was accepted: %+v", m)
	}
	s.checkReport(&TurnReport{Claims: []ReportClaim{{Text: "not achieved", Kind: "other"}}})
	if m := s.unconfirmedReport(); !m.Empty() {
		t.Fatalf("an honest report must end the turn: %+v", m)
	}
}

func callsAndResults(parts ...Block) []Message {
	var out []Message
	for _, b := range parts {
		role := "assistant"
		if b.Type == "tool_result" {
			role = "user"
		}
		out = append(out, Message{Role: role, Content: []Block{b}})
	}
	return out
}

func TestLastRunDecidesACommandClaim(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	s.Messages = append(s.Messages, callsAndResults(
		bashCall("1", "go test"), Block{Type: "tool_result", ToolUseID: "1", Content: "ok"},
		bashCall("2", "go test"), Block{Type: "tool_result", ToolUseID: "2", Content: "FAIL", IsError: true},
	)...)
	c := ReportClaim{Kind: "command", Ref: "go test"}
	s.verifyClaim(&c)
	if c.Status != "nao-confirmado" {
		t.Fatalf("a pass followed by a failure was confirmed: %+v", c)
	}
}

func TestEditAfterTheRunMakesTheClaimStale(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	s.Messages = append(s.Messages, callsAndResults(
		bashCall("1", "go test"), Block{Type: "tool_result", ToolUseID: "1", Content: "ok"},
		Block{Type: "tool_use", ID: "2", Name: "Edit", Input: json.RawMessage(`{"path":"a.txt"}`)},
		Block{Type: "tool_result", ToolUseID: "2", Content: "ok"},
	)...)
	c := ReportClaim{Kind: "command", Ref: "go test"}
	s.verifyClaim(&c)
	if c.Evidence.ID != "report.ev_stale" {
		t.Fatalf("a pass from before the last edit was confirmed: %+v", c)
	}
}

// Compaction must not disconnect Turn from TurnStarts: the Report gate, the contract and the mission
// marker all index one with the other.
func TestCompactionKeepsTurnsAddressable(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "first request")
	turnOn(t, s, "second request")
	s.Mu.Lock()
	s.adoptCompaction(compactionHead("summary", nil), []Message{{Role: "user", Content: []Block{{Type: "text", Text: "kept"}}}})
	s.Mu.Unlock()
	if len(s.TurnStarts) != s.Turn {
		t.Fatalf("turnStarts %v for turn %d", s.TurnStarts, s.Turn)
	}
	if got := s.turnRequest(); got != "second request" {
		t.Fatalf("the request was lost: %q", got)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if m := s.missingReport(); m.ID != "gate.no_report" {
		t.Fatalf("the Report gate went off after compaction: %+v", m)
	}
}

func TestContractDoesNotCrossToAnotherTurnWithTheSameWords(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "fix it")
	s.Contract = &Contract{Goal: "g", Criteria: []string{"c"}, Request: "fix it", Turn: s.Turn}
	if m := s.contractGate(); !m.Empty() {
		t.Fatalf("the confirmed contract was refused: %+v", m)
	}
	turnOn(t, s, "fix it")
	if m := s.contractGate(); m.ID != "contract.required" {
		t.Fatalf("yesterday's contract opened today's turn: %+v", m)
	}
}

// A scheduled task has nobody to answer: it cannot hang on a question, and its prompt is the request
// the user wrote in advance.
func TestUnattendedSessionNeverWaitsForAnAnswer(t *testing.T) {
	s := lockSession(t)
	s.Unattended = true
	turnOn(t, s, "nightly job")
	if m := s.contractGate(); !m.Empty() {
		t.Fatalf("an unattended turn was held by the contract: %+v", m)
	}
	done := make(chan bool, 1)
	go func() {
		_, ok := s.askUser(context.Background(), "which?", []string{"a"})
		done <- ok
	}()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("nobody answered")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the question hangs a turn nobody is watching")
	}
	if s.askPermissionWhy(context.Background(), bashTool{}, nil, "", Msg{}) {
		t.Fatal("nobody approved it")
	}
}

func TestSecondSendIsRefusedBeforeTheLoopStarts(t *testing.T) {
	s := lockSession(t)
	s.State = "thinking"
	if err := s.Send("again", nil, nil); err == nil {
		t.Fatal("a second turn started on a busy session")
	}
}

func TestIllegalStateChangeIsRefused(t *testing.T) {
	s := lockSession(t)
	s.State = "waiting_user"
	s.setState("compacting")
	if s.State != "waiting_user" {
		t.Fatalf("state %q", s.State)
	}
}

// Revert restores the bytes the turn started with, whichever tool changed them and in whatever order.
func TestRevertRestoresTheTurnsExactBytes(t *testing.T) {
	s := lockSession(t)
	p := filepath.Join(s.Root, "a.txt")
	s.Checkpoints = map[int][]CheckpointFile{}
	turnOn(t, s, "edit")
	// a shell command changes the file first; the first tool write checkpoints the changed bytes
	if err := os.WriteFile(p, []byte("shell\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.checkpointBeforeWrite(p)
	if err := os.WriteFile(p, []byte("tool\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.State = "idle"
	if err := s.Revert(1); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "one\n" {
		t.Fatalf("revert left %q", b)
	}
}

func TestRevertBringsBackAFileDeletedThenRecreated(t *testing.T) {
	s := lockSession(t)
	p := filepath.Join(s.Root, "a.txt")
	s.Checkpoints = map[int][]CheckpointFile{}
	turnOn(t, s, "edit")
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	s.checkpointBeforeWrite(p) // recorded as new: the shell had removed it
	if err := os.WriteFile(p, []byte("rewritten\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.State = "idle"
	if err := s.Revert(1); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(p); err != nil || string(b) != "one\n" {
		t.Fatalf("revert left %q %v", b, err)
	}
}

// What a cancelled turn changed is not laundered into the next turn's baseline.
func TestUnauditedTurnStaysInTheNextAudit(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	if err := os.WriteFile(filepath.Join(s.Root, "a.txt"), []byte("changed then cancelled\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.Turn++
	if err := s.turnBase(true); err != nil {
		t.Fatal(err)
	}
	changes := workspace.TurnChanges(s.Root, s.workRoot(), s.ID, s.Turn)
	if len(changes) != 1 || changes[0].Path != "a.txt" {
		t.Fatalf("the change was laundered: %+v", changes)
	}
}

func TestOnlyTheRealInfuseFolderSkipsTheProtocol(t *testing.T) {
	s := lockSession(t)
	turnOn(t, s, "edit")
	in, _ := json.Marshal(map[string]string{"path": "x.infuse/a.go", "content": "package a\n"})
	if m := s.protocolCheckWrite("Write", in); m.Empty() {
		t.Fatal("a folder merely named like .infuse skipped the plan")
	}
}
