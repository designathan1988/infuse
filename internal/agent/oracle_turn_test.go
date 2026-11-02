package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infuse/internal/oracle"
)

// The verdict is part of the definition of done: with criteria defined, a turn that changed the tree
// only ends when the executor — not the agent — approves it on the current tree.
func TestTheOracleGovernsTheEndOfTheTurn(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	noContract(s)
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.gate.planned = true
	s.gate.diagnosed = true
	// the user defines the criteria before the turn: the snapshot is the baseline the audit judges from
	spec := oracle.OracleSpec{Checks: []oracle.OracleCheck{{ID: "done", Kind: "file", Path: "out.txt"}}}
	if err := oracle.SaveSpec(root, spec); err != nil {
		t.Fatal(err)
	}
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	s.Checkpoints[1] = []CheckpointFile{{Path: "src/app.js"}}
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "app.js"), []byte("export const app = 1;\n"), 0o644)
	s.Messages = []Message{{Role: "assistant", Content: []Block{
		{Type: "tool_use", ID: "R", Name: "Report", Input: json.RawMessage(`{"summary":"done","claims":[{"text":"created the app","kind":"other"}]}`)},
	}}}
	s.settings.RequireReview = boolPtr(false)
	why := s.turnObligation(context.Background())
	if why.Empty() || !strings.Contains(why.Render("en"), "out.txt") {
		t.Fatalf("the failing criterion must block the turn end, got %q", why.Render("en"))
	}
	// the agent's own fix makes the criterion pass; the executor runs again and approves
	_ = os.WriteFile(filepath.Join(root, "out.txt"), []byte("done\n"), 0o644)
	if why := s.oracleObligation(context.Background()); !why.Empty() {
		t.Fatalf("a passing criterion must end the obligation: %s", why.Render("en"))
	}
}

func boolPtr(b bool) *bool { return &b }
