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

func waitMission(t *testing.T, s *Session, states ...string) *Mission {
	t.Helper()
	for i := 0; i < 1500; i++ {
		if m := s.missionSnapshot(); m != nil {
			for _, st := range states {
				if m.State == st {
					return m
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("mission did not reach %v: %+v", states, s.missionSnapshot())
	return nil
}

func TestMissionRunsUntilCriteriaPassOrBudget(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"file","kind":"file","path":"a.txt","expect":{"contains":"ok"}}]}`)
	s := newSession(root)
	s.Provider = "mock"

	// criteria never met by the (scripted) agent: the mission stops at the turn limit, with each verdict kept
	if err := s.StartMission("create a.txt with ok", 5, 2); err != nil {
		t.Fatal(err)
	}
	m := waitMission(t, s, "orcamento")
	if len(m.Steps) != 2 || m.Steps[0].Verdict != "reprovado" || m.Steps[0].Run == "" {
		t.Fatalf("%+v", m)
	}
	// the agent was sent back with the failing criterion; the messages say they are the mission's by data
	found, brief := false, false
	for _, msg := range s.Messages {
		n, ok := missionNoteOf(msg)
		if !ok {
			continue
		}
		if n.Goal == "create a.txt with ok" && n.Turn == 0 && strings.HasSuffix(msgText(msg), "create a.txt with ok") {
			brief = true
		}
		if n.Turn == 2 && n.MaxTurns == 2 && n.Verdict == "reprovado" && strings.Contains(msgText(msg), "file:") && strings.HasPrefix(msgText(msg), "[Mission — turn 2 of 2") {
			found = true
		}
	}
	if !brief || !found {
		t.Fatalf("brief %v, next turn with the failing criteria %v: %+v", brief, found, s.Messages)
	}
	if s.Title != "create a.txt with ok" {
		t.Fatalf("title %q", s.Title)
	}

	// once the criterion holds, the mission ends approved after one turn
	waitIdle(t, s)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("ok\n"), 0o644)
	if err := s.StartMission("confirm", 5, 5); err != nil {
		t.Fatal(err)
	}
	if m := waitMission(t, s, "aprovada"); len(m.Steps) != 1 {
		t.Fatalf("%+v", m)
	}
}

func TestImpasseEndsTheMission(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mission = &Mission{Goal: "x", State: "running", BudgetUSD: 1, MaxTurns: 3}
	out, isErr := impasseTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"reason":"no access to the database","tried":["connect with the credentials from .env"],"evidence":"ECONNREFUSED","needs":"the test database URL"}`))
	if isErr {
		t.Fatal(out)
	}
	m := s.missionSnapshot()
	if m.State != "impasse" || m.Impasse == nil || m.Impasse.Needs != "the test database URL" {
		t.Fatalf("%+v", m)
	}
}

// Stopping a mission cancels the verification it left running; a late step cannot resurrect it.
func TestEndingAMissionCancelsItsVerification(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Mission = &Mission{Goal: "x", State: "running", BudgetUSD: 1, MaxTurns: 3}
	cancelled := false
	s.missionCancel = func() { cancelled = true }
	s.endMission("parada", "stopped by the user")
	if !cancelled || s.missionSnapshot().State != "parada" {
		t.Fatalf("cancelled %v, mission %+v", cancelled, s.missionSnapshot())
	}
}
