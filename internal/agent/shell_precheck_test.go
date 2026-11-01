package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A heredoc's content is known before the command runs: the write policy must judge it then, not only
// at the end of the turn.
func TestHeredocIsJudgedBeforeItRuns(t *testing.T) {
	s, root := shellProject(t)
	s.gate.planned = false
	in := shellWriteIntent{Abs: filepath.Join(root, "src", "new.js"), Content: "export const added = 1;\n"}
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); why.ID != "gate.shell_write_plan" {
		t.Fatalf("heredoc source write without a plan: %q", why.Render("en"))
	}
}

func TestHeredocRespectsAPathBan(t *testing.T) {
	s, root := shellProject(t)
	s.setBans([]Ban{{Kind: "path", Value: "src/legacy", Quote: "do not touch src/legacy", Turn: 1}})
	in := shellWriteIntent{Abs: filepath.Join(root, "src", "legacy", "x.js"), Content: "export const x = 1;\n"}
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); why.ID != "ban.refused" {
		t.Fatalf("banned path written by heredoc: %q", why.Render("en"))
	}
}

func TestHeredocSuppressorNeedsAReason(t *testing.T) {
	s, root := shellProject(t)
	in := shellWriteIntent{Abs: filepath.Join(root, "src", "quiet.js"), Content: "export const x = 1;\n// eslint-disable\n"}
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); why.ID != "shortcut.justify" {
		t.Fatalf("suppressor introduced by heredoc: %q", why.Render("en"))
	}
}

func TestHeredocCleanWritePasses(t *testing.T) {
	s, root := shellProject(t)
	in := shellWriteIntent{Abs: filepath.Join(root, "src", "clean.js"), Content: "export const clean = 4;\n"}
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); !why.Empty() {
		t.Fatalf("clean heredoc refused: %q", why.Render("en"))
	}
}

// The shell audit applies the same prohibitions, suppressors and test-infrastructure rule as Write.
func TestShellAuditAppliesBansSuppressorsAndTestInfra(t *testing.T) {
	s, root := shellProject(t)
	s.setBans([]Ban{{Kind: "path", Value: "src/legacy", Quote: "do not touch src/legacy", Turn: 1}})
	writeFile(t, root, "src/legacy/x.js", "export const x = 1;\n")
	if why := s.auditShellWrites(); !strings.Contains(why.Render("en"), "src/legacy/x.js") {
		t.Fatalf("shell write to a banned path: %q", why.Render("en"))
	}
	if err := os.Remove(filepath.Join(root, "src", "legacy", "x.js")); err != nil {
		t.Fatal(err)
	}
	s.setBans(nil)
	writeFile(t, root, "src/quiet.js", "export const x = 1;\n// eslint-disable\n")
	if why := s.auditShellWrites(); !strings.Contains(why.Render("en"), "src/quiet.js") {
		t.Fatalf("shell suppressor: %q", why.Render("en"))
	}
	if err := os.Remove(filepath.Join(root, "src", "quiet.js")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, "conftest.py", "value = 1\n")
	if why := s.auditShellWrites(); !strings.Contains(why.Render("en"), "conftest.py") {
		t.Fatalf("test infrastructure changed by shell: %q", why.Render("en"))
	}
}

// Without the snapshot there is no record of what changed: the turn cannot end.
func TestSnapshotFailureBlocksTheTurn(t *testing.T) {
	s, _ := shellProject(t)
	s.snapshotErr = errors.New("disk full")
	if why := s.turnObligation(context.Background()); why.ID != "turn.snapshot" {
		t.Fatalf("missing snapshot did not block: %q", why.Render("en"))
	}
	if !strings.Contains(M("turn.unverified").Render("en"), "UNVERIFIED") {
		t.Fatal("the unverified message lost its meaning")
	}
}
