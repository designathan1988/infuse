package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A script that writes the oracle's criteria directly is caught at the end of the turn, even though
// the shell guard only reads the command line.
func TestOracleFilesChangedOutsideTheExecutorAreRefused(t *testing.T) {
	s, root := shellProject(t)
	oracleDir := filepath.Join(root, ".infuse", "oracle")
	_ = os.MkdirAll(oracleDir, 0o755)
	_ = os.WriteFile(filepath.Join(oracleDir, "checks.json"), []byte(`{"checks":[]}`), 0o644)
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	_ = os.WriteFile(filepath.Join(oracleDir, "checks.json"), []byte(`{"checks":[{"id":"x","kind":"file","path":"out.txt"}]}`), 0o644)
	why := s.auditShellWrites()
	if why.Empty() || !strings.Contains(why.Render("en"), ".infuse/oracle/checks.json") {
		t.Fatalf("a criteria write outside the executor must be refused, got %q", why.Render("en"))
	}
}

// B6: a new exit(0) outside the entry point turns any failure into apparent success.
func TestExitZeroOutsideTheEntryPointIsRefused(t *testing.T) {
	before := "export function run() {\n  return 1;\n}\n"
	after := "export function run() {\n  process.exit(0);\n}\n"
	if m := exitZeroCheck("src/run.js", before, after); m.Empty() {
		t.Fatal("exit(0) in a library file must be refused")
	}
	if m := exitZeroCheck("index.js", before, after); !m.Empty() {
		t.Fatal("the entry point may end the process")
	}
}

// B5: a test run whose output is hidden cannot be read.
func TestSilencedTestOutputIsRefused(t *testing.T) {
	if m := swallowedTestCommand("npm test >/dev/null"); m.Empty() {
		t.Fatal("hiding a test runner's output must be refused")
	}
	if m := swallowedTestCommand("npm test 2>/dev/null"); !m.Empty() {
		t.Fatal("silencing only stderr keeps the result readable")
	}
}

// An unreadable criteria file blocks the turn instead of turning the executor off.
func TestUnreadableCriteriaBlockTheTurn(t *testing.T) {
	s, root := shellProject(t)
	_ = os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".infuse", "oracle", "checks.json"), []byte("{not json"), 0o644)
	if why := s.oracleObligation(t.Context()); why.Empty() || why.ID != "turn.oracle_spec" {
		t.Fatalf("a broken criteria file must block, got %q", why.Render("en"))
	}
}

// A shell deletion that removes most of a file is grounded in the plan or diagnosis, like a Write.
func TestLargeShellRemovalNeedsThePlanOrDiagnosis(t *testing.T) {
	s, root := shellProject(t)
	body := strings.Repeat("export const value = 1;\n", 60)
	writeFile(t, root, "src/whole.js", body)
	snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn)
	if err := os.Remove(filepath.Join(root, "src", "whole.js")); err != nil {
		t.Fatal(err)
	}
	if why := s.auditShellWrites(); why.Empty() {
		t.Fatal("removing a whole file outside the write tools must be grounded")
	}
}
