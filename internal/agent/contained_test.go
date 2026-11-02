package agent

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"infuse/internal/mcp"
	"infuse/internal/oracle"
	"infuse/internal/sandbox"
)

func mcpServerConfig(t *testing.T, probe string) mcp.ServerConfig {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return mcp.ServerConfig{Command: exe, Args: []string{"-test.run=^$"}, Env: map[string]string{"INFUSE_FAKE_MCP": "1", "INFUSE_FAKE_MCP_PROBE": probe}}
}

// A server the project defines runs like the agent's shell: it starts, serves, and cannot write outside
// the project. The user's own global server stays outside containment, which also proves the probe.
func TestMCPServerOfTheProjectRunsContained(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("this platform has no OS containment")
	}
	testHome(t)
	root := t.TempDir()
	probe := filepath.Join(t.TempDir(), "escaped.txt")
	cfg := mcpServerConfig(t, probe)
	if err := editMCP(root, "project", func(sv map[string]mcp.ServerConfig) { sv["fake"] = cfg }); err != nil {
		t.Fatal(err)
	}
	m := mcpFor(root)
	defer mcp.CloseAll()
	if err := mcp.Approve(root, "project", "fake", cfg); err != nil {
		t.Fatal(err)
	}
	m.Ensure(context.Background())
	if len(m.Tools()) != 2 {
		t.Fatalf("a contained server must still serve: %+v", m.Status())
	}
	if _, err := os.Stat(probe); err == nil {
		t.Fatal("a project's MCP server wrote outside the project")
	}
}

func TestMCPServerOfTheUserIsNotContained(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	probe := filepath.Join(t.TempDir(), "user.txt")
	cfg := mcpServerConfig(t, probe)
	if err := editMCP(root, "global", func(sv map[string]mcp.ServerConfig) { sv["fake"] = cfg }); err != nil {
		t.Fatal(err)
	}
	m := mcpFor(root)
	defer mcp.CloseAll()
	if err := mcp.Approve(root, "global", "fake", cfg); err != nil {
		t.Fatal(err)
	}
	m.Ensure(context.Background())
	if len(m.Tools()) != 2 {
		t.Fatalf("%+v", m.Status())
	}
	if _, err := os.Stat(probe); err != nil {
		t.Fatalf("the probe itself does not work: %v", err)
	}
}

// A py_compile.py beside the file must not be what the diagnostics run.
func TestDiagnosticsDoNotImportFromTheFilesFolder(t *testing.T) {
	if _, err := exec.LookPath("python"); err != nil {
		t.Skip("python is not installed")
	}
	testHome(t)
	root := t.TempDir()
	marker := filepath.Join(t.TempDir(), "planted-ran")
	planted := "open(" + strconvQuote(marker) + ", 'w').write('x')\n"
	if err := os.WriteFile(filepath.Join(root, "py_compile.py"), []byte(planted), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "app.py")
	if err := os.WriteFile(src, []byte("def broken(:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := fileDiagnostics(root, src)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a planted module ran on the host")
	}
	if !strings.Contains(d, "SyntaxError") {
		t.Fatalf("the real checker must still report the error: %q", d)
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// The project's own tsc is agent-authored code: it runs contained.
func TestProjectCheckerRunsContained(t *testing.T) {
	if runtime.GOOS != "windows" || !sandbox.Available() {
		t.Skip("the fake checker is a Windows command script")
	}
	testHome(t)
	root := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped.txt")
	bin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "@echo off\r\necho escaped> \"" + escaped + "\"\r\necho app.ts(1,1): error TS1000: fake\r\nexit /b 1\r\n"
	if err := os.WriteFile(filepath.Join(bin, "tsc.cmd"), []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "app.ts")
	if err := os.WriteFile(src, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := fileDiagnostics(root, src)
	if _, err := os.Stat(escaped); err == nil {
		t.Fatal("the project's checker wrote outside the project")
	}
	if !strings.Contains(d, "TS1000") {
		t.Fatalf("the checker must still run and report: %q", d)
	}
}

// turnWithCriteria is a session with saved criteria at the start of a turn.
func turnWithCriteria(t *testing.T) (*Session, string) {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := oracle.OracleSpec{Checks: []oracle.OracleCheck{{ID: "a", Kind: "file", Path: "a.txt"}}}
	if err := oracle.SaveSpec(root, spec); err != nil {
		t.Fatal(err)
	}
	s := newSession(root)
	s.Turn = 1
	s.TurnStarts = []int{0}
	if err := snapshotTurn(s.Root, s.workRoot(), s.ID, s.Turn); err != nil {
		t.Fatal(err)
	}
	return s, root
}

// Running the executor must not launder a changed criteria file into the turn's baseline.
func TestExecutorRunDoesNotAcceptChangedCriteria(t *testing.T) {
	s, root := turnWithCriteria(t)
	forged := `{"checks":[{"id":"a","kind":"file","path":"a.txt","expect":{"exists":true}}]}`
	if err := os.WriteFile(SpecFile(root), []byte(forged), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = s.oracleObligation(context.Background())
	if why := s.auditShellWrites(); why.Empty() || !strings.Contains(why.Render("en"), "checks.json") {
		t.Fatalf("the forged criteria were accepted after a run: %q", why.Render("en"))
	}
}

// The executor's own evidence is accepted, and only that: the audit stays quiet after a clean run.
func TestExecutorEvidenceIsNotBlamedOnTheAgent(t *testing.T) {
	s, _ := turnWithCriteria(t)
	if why := s.oracleObligation(context.Background()); !why.Empty() {
		t.Fatal(why.Render("en"))
	}
	if p := s.protectedChanges(); len(p) != 0 {
		t.Fatalf("%v", p)
	}
}

// Criteria the user saves while a turn runs are the user's: they match the anchor.
func TestCriteriaSavedByTheUserMidTurnAreNotTheAgents(t *testing.T) {
	s, root := turnWithCriteria(t)
	spec := oracle.OracleSpec{Checks: []oracle.OracleCheck{{ID: "b", Kind: "file", Path: "a.txt"}}}
	if err := oracle.SaveSpec(root, spec); err != nil {
		t.Fatal(err)
	}
	if p := s.protectedChanges(); len(p) != 0 {
		t.Fatalf("%v", p)
	}
}

// Deleting the criteria file cannot disarm the oracle: the end of the turn refuses instead of reading
// the absence as "no criteria".
func TestDeletedCriteriaBlockTheTurn(t *testing.T) {
	s, root := turnWithCriteria(t)
	if err := os.Remove(SpecFile(root)); err != nil {
		t.Fatal(err)
	}
	if why := s.oracleObligation(context.Background()); why.Empty() {
		t.Fatal("a turn ended with the user's criteria deleted")
	}
	if p := s.protectedChanges(); len(p) != 1 {
		t.Fatalf("the deletion must be reported as a protected change: %v", p)
	}
}

// A patched dependency moves the verdict's key, so an approval is never replayed over it.
func TestPatchedDependencyDoesNotReplayAVerdict(t *testing.T) {
	s, root := turnWithCriteria(t)
	dep := filepath.Join(root, "node_modules", ".bin", "jest")
	if err := os.MkdirAll(filepath.Dir(dep), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dep, []byte("exit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := s.oracleRunKey()
	if err := os.WriteFile(dep, []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s.oracleRunKey() == before {
		t.Fatal("the cached verdict would replay over a patched dependency")
	}
}
