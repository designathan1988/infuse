package agent

import (
	"context"
	"infuse/internal/oracle"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// answerNext answers the next user question the session asks, as the UI would.
func answerNext(s *Session, answer string) {
	go func() {
		for i := 0; i < 300; i++ {
			s.Mu.Lock()
			req := s.pendingAsk
			s.Mu.Unlock()
			if req != nil {
				s.Answer(req.ID, answer)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
}

// A delivery whose criteria are marked high-stakes asks the user once; accepting lets the turn end and
// rejecting ends it as not accepted.
func TestAcceptanceGateAsksOnceForHighStakesCriteria(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := oracle.SaveSpec(root, oracle.OracleSpec{RequireAcceptance: true}); err != nil {
		t.Fatal(err)
	}
	s := newSession(root)
	changed := []CheckpointFile{{Path: "src/a.js"}}
	accept := M("turn.accept").Render(s.uiLang())
	answerNext(s, accept)
	if why := s.acceptanceGate(context.Background(), changed); !why.Empty() {
		t.Fatalf("accepted delivery refused: %q", why.Render("en"))
	}
	s.Mu.Lock()
	asked := s.gate.acceptanceAsked
	s.Mu.Unlock()
	if !asked {
		t.Fatal("the acceptance question was not recorded")
	}
	if why := s.acceptanceGate(context.Background(), changed); !why.Empty() {
		t.Fatal("the gate asked again in the same turn")
	}

	// A plain spec never asks.
	plain := newSession(t.TempDir())
	if why := plain.acceptanceGate(context.Background(), changed); !why.Empty() {
		t.Fatal("a spec without requireAcceptance asked")
	}
	if plain.pendingAsk != nil {
		t.Fatal("a question was left pending")
	}
}

// A rejected delivery ends the turn and leaves a journal record.
func TestRejectedDeliveryIsRecorded(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := oracle.SaveSpec(root, oracle.OracleSpec{RequireAcceptance: true}); err != nil {
		t.Fatal(err)
	}
	s := newSession(root)
	answerNext(s, M("turn.reject").Render(s.uiLang()))
	if why := s.acceptanceGate(context.Background(), []CheckpointFile{{Path: "src/a.js"}}); !why.Empty() {
		t.Fatalf("rejection must end the turn quietly: %q", why.Render("en"))
	}
	b, _ := os.ReadFile(filepath.Join(root, ".infuse", "journal.jsonl"))
	if !strings.Contains(string(b), "rejected by the user") {
		t.Fatal("the rejection was not journaled")
	}
}
