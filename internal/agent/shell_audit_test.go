package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shellProject(t *testing.T) (*Session, string) {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "base.js"), []byte("export const base = 1;\n"), 0o644)
	s := newSession(root)
	s.Turn = 1
	s.gate.planned = true
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	return s, root
}

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(rel))
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A file written by a shell command is judged by the same rules as a Write: the turn cannot end on a
// file over the size limit, whoever wrote it.
func TestShellWriteIsAuditedBeforeTheTurnEnds(t *testing.T) {
	s, root := shellProject(t)
	writeFile(t, root, "src/big.js", strings.Repeat("const x = someValue;\n", 500))
	why := s.auditShellWrites()
	if why.Empty() {
		t.Fatal("a shell write over the file limit must be refused")
	}
	if !strings.Contains(why.Render("en"), "src/big.js") {
		t.Fatalf("the refusal must name the file: %s", why.Render("en"))
	}
	_ = os.Remove(filepath.Join(root, "src", "big.js"))
	writeFile(t, root, "src/ok.js", "export const ok = 2;\n")
	if why := s.auditShellWrites(); !why.Empty() {
		t.Fatalf("a clean shell write must pass: %s", why.Render("en"))
	}
}

// The shell is not a way around the plan: source changed outside the write tools needs a plan.
func TestShellWriteNeedsAPlan(t *testing.T) {
	s, root := shellProject(t)
	s.gate.planned = false
	writeFile(t, root, "src/new.js", "export const added = 3;\n")
	why := s.auditShellWrites()
	if why.Empty() || why.ID != "gate.shell_write_plan" {
		t.Fatalf("a shell source write without a plan must be refused, got %q", why.Render("en"))
	}
}

// A path the write gates accepted is not audited again. A checkpoint alone is not proof: a heredoc
// also records one before the command runs, and that file never passed a gate.
func TestGatedWritesAreNotAuditedTwice(t *testing.T) {
	s, root := shellProject(t)
	s.markGated("src/big.js")
	writeFile(t, root, "src/big.js", strings.Repeat("const x = someValue;\n", 500))
	if why := s.auditShellWrites(); !why.Empty() {
		t.Fatalf("a write tool's own path must not be judged again here: %s", why.Render("en"))
	}
	// a checkpoint with no gate (the filter's heredoc) is still judged
	s, root = shellProject(t)
	s.Checkpoints[s.Turn] = []CheckpointFile{{Path: "src/heredoc.js"}}
	writeFile(t, root, "src/heredoc.js", strings.Repeat("const y = someValue;\n", 500))
	if why := s.auditShellWrites(); why.Empty() {
		t.Fatal("a heredoc write is checkpointed but not gated: it must be judged")
	}
}

// Deleting a file is a change the audit sees, but not content to judge.
func TestShellDeletionEndsTheTurnClean(t *testing.T) {
	s, root := shellProject(t)
	if err := os.Remove(filepath.Join(root, "src", "base.js")); err != nil {
		t.Fatal(err)
	}
	if why := s.auditShellWrites(); !why.Empty() {
		t.Fatalf("a deletion is not a content violation: %s", why.Render("en"))
	}
	changes := s.ungatedChanges()
	if len(changes) != 1 || !changes[0].Deleted {
		t.Fatalf("the deletion must be reported as a change, got %+v", changes)
	}
}
