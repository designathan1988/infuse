package agent

import (
	"infuse/internal/workspace"
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
	writeFile(t, root, "src/big.js", strings.Repeat("const x = someValue;\n", 500))
	s.markGated("src/big.js") // the digest the successful write tool left
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

// The instructions bind the shell too: a rewrite is refused and the original bytes are restored.
func TestShellCannotRewriteTheProjectInstructions(t *testing.T) {
	s, root := shellProject(t)
	original := "# Rules\n\nDo not rewrite me.\n"
	writeFile(t, root, "CLAUDE.md", original)
	s.Turn = 2
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	writeFile(t, root, "CLAUDE.md", "# Rules\n\nAnything goes now.\n")
	why := s.auditShellWrites()
	if why.Empty() || !strings.Contains(why.Render("en"), "CLAUDE.md") {
		t.Fatalf("a shell rewrite of the instructions passed: %q", why.Render("en"))
	}
	if b, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md")); string(b) != original {
		t.Fatalf("the instructions were not restored: %q", b)
	}
}

// The write gate approves a content, not a path: a later shell rewrite of the same test is audited
// and restored.
func TestGatedPathRewrittenByTheShellIsAudited(t *testing.T) {
	s, root := shellProject(t)
	testPath := "src/sum.test.js"
	original := "test('sums', () => { assert(1 + 1 === 2); });\n"
	writeFile(t, root, testPath, original)
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	s.markGated(testPath)
	writeFile(t, root, testPath, "test.skip('sums', () => {});\n")
	if why := s.auditShellWrites(); why.Empty() {
		t.Fatal("a shell rewrite of a gated test passed the audit")
	}
	if b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(testPath))); string(b) != original {
		t.Fatalf("the weakened test was not restored: %q", b)
	}
}

// The shell is not a way around the user's tests: a weakening change is refused and the original
// bytes are put back; an added test is legitimate.
func TestShellTamperingWithATestIsRestored(t *testing.T) {
	s, root := shellProject(t)
	testPath := filepath.Join(root, "src", "sum.test.js")
	original := "test('sums', () => { assert(1 + 1 === 2); });\n"
	writeFile(t, root, "src/sum.test.js", original)
	snapshotTurn(s.Root, s.workRoot(), s.ID, 2)
	s.Turn = 2
	writeFile(t, root, "src/sum.test.js", "test.skip('sums', () => {});\n")
	why := s.auditShellWrites()
	if why.Empty() || !strings.Contains(why.Render("en"), "src/sum.test.js") {
		t.Fatalf("a shell test change was not refused: %q", why.Render("en"))
	}
	if b, _ := os.ReadFile(testPath); string(b) != original {
		t.Fatalf("the test was not restored: %q", b)
	}

	snapshotTurn(s.Root, s.workRoot(), s.ID, 3)
	s.Turn = 3
	writeFile(t, root, "src/sum.test.js", original+"test('adds zero', () => { assert(0 + 0 === 0); });\n")
	if why := s.auditShellWrites(); !why.Empty() {
		t.Fatalf("an added test through the shell was refused: %q", why.Render("en"))
	}
	writeFile(t, root, "src/extra.test.js", "test('extra', () => {});\n")
	if why := s.auditShellWrites(); !why.Empty() || !s.agentFile("src/extra.test.js") {
		t.Fatalf("a new shell test was not accepted as the agent's: %q", why.Render("en"))
	}
}

// Deleting a control record is always a violation, even though the deleted file is gone.
func TestShellDeletionOfProtectedRecordIsRestored(t *testing.T) {
	s, root := shellProject(t)
	oracle := filepath.Join(root, ".infuse", "oracle", "checks.json")
	writeFile(t, root, ".infuse/oracle/checks.json", "{\"checks\":[]}\n")
	snapshotTurn(s.Root, s.workRoot(), s.ID, 2)
	s.Turn = 2
	if err := os.Remove(oracle); err != nil {
		t.Fatal(err)
	}
	why := s.auditShellWrites()
	if why.Empty() || !strings.Contains(why.Render("en"), ".infuse/oracle/checks.json") {
		t.Fatalf("protected deletion not refused: %q", why.Render("en"))
	}
	if b, err := os.ReadFile(oracle); err != nil || string(b) != "{\"checks\":[]}\n" {
		t.Fatalf("protected record not restored: %q %v", b, err)
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

// A control file modified by the shell is refused and restored, not only a deleted one.
func TestShellCannotModifyProtectedRecords(t *testing.T) {
	s, root := shellProject(t)
	original := `{"mcpServers":{}}` + "\n"
	writeFile(t, root, ".mcp.json", original)
	s.Turn = 2
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	writeFile(t, root, ".mcp.json", `{"mcpServers":{"evil":{"command":"cmd"}}}`+"\n")
	why := s.auditShellWrites()
	if why.Empty() {
		t.Fatal("a shell rewrite of .mcp.json passed the audit")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".mcp.json")); string(b) != original {
		t.Fatalf("the control file was not restored: %q", b)
	}
}

// Deleting a baseline is a change like any other (and is restored).
func TestShellCannotDeleteABaseline(t *testing.T) {
	s, root := shellProject(t)
	original := "{\"count\": 3}\n"
	writeFile(t, root, "coverage.json", original)
	s.Turn = 2
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	if err := os.Remove(filepath.Join(root, "coverage.json")); err != nil {
		t.Fatal(err)
	}
	why := s.auditShellWrites()
	if why.Empty() || !strings.Contains(why.Render("en"), "coverage.json") {
		t.Fatalf("a deleted baseline passed the audit: %q", why.Render("en"))
	}
	if b, _ := os.ReadFile(filepath.Join(root, "coverage.json")); string(b) != original {
		t.Fatalf("the baseline was not restored: %q", b)
	}
}

// A screen edited through the shell still owes a screenshot.
func TestShellUIChangeNeedsAScreenshot(t *testing.T) {
	s, root := shellProject(t)
	writeFile(t, root, "src/App.tsx", "export function App() { return <h1>Old</h1>; }\n")
	s.Turn = 2
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	writeFile(t, root, "src/App.tsx", "export function App() { return <h1>New</h1>; }\n")
	if why := s.auditShellWrites(); !why.Empty() {
		t.Fatalf("a plain shell UI change was refused: %q", why.Render("en"))
	}
	if why := s.missingScreenshot(); why.Empty() {
		t.Fatal("a screen changed through the shell demanded no screenshot")
	}
}

// A patched installed library blocks the turn end unless an install command rebased the fingerprint.
func TestInstalledDependencyChangeNeedsAnInstall(t *testing.T) {
	s, root := shellProject(t)
	writeFile(t, root, "node_modules/jest-runner/index.js", "module.exports = () => 0;\n")
	s.Turn = 2
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	writeFile(t, root, "node_modules/jest-runner/index.js", "module.exports = () => 0; // patched\n")
	if why := s.dependencyObligation(); why.ID != "turn.dependencies" {
		t.Fatalf("a patched dependency passed the turn end: %q", why.Render("en"))
	}
	workspace.RefreshTurnDependencies(s.Root, s.ID, s.Turn) // what a successful install does
	if why := s.dependencyObligation(); !why.Empty() {
		t.Fatalf("the approved install did not rebase the dependency: %q", why.Render("en"))
	}
}
