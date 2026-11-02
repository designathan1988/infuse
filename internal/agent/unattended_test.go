package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// A mission or a scheduled run has no human present: the card would wait forever, so the call is
// allowed and a question takes its first option. The deterministic gates still run.
func TestUnattendedSessionAnswersItsOwnCards(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Unattended = true
	s.settings.Permissions = map[string]string{"Bash": "ask"}
	if got := s.permissionFor(bashTool{}, json.RawMessage(`{"command":"npm test"}`)); got != "allow" {
		t.Fatalf("unattended Bash decision: %q", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ans, ok := s.askUser(ctx, "confirm the contract?", []string{"Confirm", "Change"})
	if !ok || ans != "Confirm" {
		t.Fatalf("unattended question: %q %v", ans, ok)
	}
}

// Plan mode is a deterministic refusal, not a question: an unattended run must not bypass it.
func TestUnattendedKeepsPlanModeDenyingWrites(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Unattended, s.Mode = true, "plan"
	if got := s.permissionFor(writeTool{}, json.RawMessage(`{"path":"a.txt","content":"x"}`)); got != "deny" {
		t.Fatalf("plan mode allowed a write in an unattended run: %q", got)
	}
}

// A mission ends unattended and the session asks again for the user's own turns.
func TestMissionTurnsUnattendedOffWhenItEnds(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"file","kind":"file","path":"a.txt","expect":{"contains":"ok"}}]}`)
	s := newSession(root)
	s.Provider = "mock"
	if err := s.StartMission("create a.txt with ok", 5, 1); err != nil {
		t.Fatal(err)
	}
	if !s.Unattended {
		t.Fatal("a running mission must be unattended")
	}
	waitMission(t, s, "orcamento")
	if s.Unattended {
		t.Fatal("an ended mission left the session unattended")
	}
}
