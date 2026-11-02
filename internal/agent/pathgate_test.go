package agent

// The path gates of the tools: one canonical spelling, the real place of a path, hidden records from
// every starting point, the project's policy inside a worktree, and a journal entry for every attempt.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// worktreeSession is a session working in a worktree kept in the default folder.
func worktreeSession(t *testing.T) (*Session, *ToolContext, string, string) {
	t.Helper()
	s, tc, _ := oracleSession(t)
	root := s.Root
	work := filepath.Join(root, ".infuse", "worktrees", "w")
	writeFile(t, work, "src/a.txt", "needle in the worktree\n")
	writeFile(t, root, ".infuse/journal.jsonl", "{\"kind\":\"needle\"}\n")
	writeFile(t, root, ".infuse/sessions/s.json", "needle\n")
	writeFile(t, root, ".infuse/worktrees/other/b.txt", "needle in another worktree\n")
	s.Worktree, s.Cwd = work, work
	tc.Root, tc.Cwd = work, work
	return s, tc, root, work
}

func TestWriteRefusesWindowsSpellings(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows path spellings")
	}
	_, tc, oracle := oracleSession(t)
	root := tc.Root
	for _, p := range []string{
		".mcp.json.", ".mcp.json ", ".mcp.json::$DATA", ".infuse/hooks.json ", ".infuse./evidence/x",
		".infuse::$INDEX_ALLOCATION/oracle/ready.md", `\\?\` + oracle, `\\.\` + oracle,
	} {
		out, isErr := run(t, tc, "Write", map[string]any{"path": p, "content": "hack"})
		if !isErr {
			t.Errorf("%q was written", p)
		}
		_ = out
	}
	if b, _ := os.ReadFile(oracle); string(b) != "ORIGINAL\n" {
		t.Fatal("the oracle changed")
	}
	if _, err := os.Stat(filepath.Join(root, ".infuse", "evidence")); err == nil {
		t.Fatal("the evidence folder was created through a spelling")
	}
	for _, p := range []string{`\\server\share\x.txt`, `\\?\UNC\server\share\x.txt`, `\\?\GLOBALROOT\Device\HarddiskVolume3\x.txt`} {
		if _, err := tc.resolvePath(p, true); err == nil {
			t.Errorf("%q passed the write gate", p)
		}
	}
}

func TestWriteProtectsTheNewControlFiles(t *testing.T) {
	_, tc, _ := oracleSession(t)
	for _, p := range []string{".infuse/rules.json", ".infuse/launch.json", ".infuse/commands/x.md", ".infuse/skills/s/SKILL.md", ".git/config", ".git/hooks/pre-commit"} {
		if _, isErr := run(t, tc, "Write", map[string]any{"path": p, "content": "hack"}); !isErr {
			t.Errorf("%s was written", p)
		}
	}
	out, _ := run(t, tc, "Write", map[string]any{"path": ".git/hooks/pre-commit", "content": "hack"})
	if !strings.Contains(out, "git runs") {
		t.Fatalf("the refusal does not explain itself: %q", out)
	}
	if _, isErr := run(t, tc, "Write", map[string]any{"path": ".infuse/docs/pinned.md", "content": "ok"}); isErr {
		t.Fatal("pinned docs stay writable")
	}
}

func TestWriteResolvesLinksAtTheDeepestAncestor(t *testing.T) {
	_, tc, oracle := oracleSession(t)
	root := tc.Root
	outside := t.TempDir()
	if err := os.Symlink(filepath.Dir(oracle), filepath.Join(root, "inner")); err != nil {
		t.Skip("symlinks not available: " + err.Error())
	}
	if err := os.Symlink(outside, filepath.Join(root, "outer")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"inner/new.md", "inner/a/b/new.md", "outer/x.txt", "outer/a/b/x.txt"} {
		if _, isErr := run(t, tc, "Write", map[string]any{"path": p, "content": "hack"}); !isErr {
			t.Errorf("%s was written", p)
		}
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("a write left the project through a link")
	}
}

// resolvePath used to leave out-of-root reads unjudged: a worktree is three ../ away from the records.
func TestReadsReachNoHiddenRecordFromAWorktree(t *testing.T) {
	_, tc, root, _ := worktreeSession(t)
	for _, p := range []string{"../../../.infuse/journal.jsonl", "../../sessions/s.json", "../other/b.txt", filepath.Join(root, ".infuse", "journal.jsonl")} {
		if out, isErr := run(t, tc, "Read", map[string]any{"path": p}); !isErr {
			t.Errorf("Read %s: %q", p, out)
		}
		if out, isErr := run(t, tc, "Grep", map[string]any{"pattern": "needle", "path": p}); !isErr {
			t.Errorf("Grep %s: %q", p, out)
		}
	}
	if out, isErr := run(t, tc, "Read", map[string]any{"path": "src/a.txt"}); isErr {
		t.Fatalf("own file: %q", out)
	}
}

func TestGrepAndGlobWalkPastHiddenRecords(t *testing.T) {
	_, tc, _, _ := worktreeSession(t)
	out, _ := run(t, tc, "Grep", map[string]any{"pattern": "needle", "path": "../../.."})
	if strings.Contains(out, "journal") || strings.Contains(out, "sessions") || strings.Contains(out, "another worktree") {
		t.Fatalf("Grep reached a record: %q", out)
	}
	out, _ = run(t, tc, "Glob", map[string]any{"pattern": "**/*", "path": "../../.."})
	if strings.Contains(out, "journal") || strings.Contains(out, "sessions") {
		t.Fatalf("Glob reached a record: %q", out)
	}
	out, _ = run(t, tc, "Grep", map[string]any{"pattern": "needle"})
	if !strings.Contains(out, "src/a.txt") {
		t.Fatalf("the worktree's own text was hidden: %q", out)
	}
	if out, isErr := run(t, tc, "LS", map[string]any{"path": "../../.."}); isErr || strings.Contains(out, ".infuse") {
		t.Fatalf("LS listed the records: %q", out)
	}
}

func TestProjectPolicyAppliesInTheWorktree(t *testing.T) {
	s, tc, _, work := worktreeSession(t)
	s.settings.ProtectedPaths = []string{"secrets/**"}
	writeFile(t, work, "CLAUDE.md", "follow these\n")
	for _, p := range []string{"CLAUDE.md", "secrets/key"} {
		if _, isErr := run(t, tc, "Write", map[string]any{"path": p, "content": "hack"}); !isErr {
			t.Errorf("%s was written in the worktree", p)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(work, "CLAUDE.md")); string(b) != "follow these\n" {
		t.Fatal("the instructions changed")
	}
}

func TestHeredocInTheBashToolLandsWhereItWasJudged(t *testing.T) {
	_, tc, oracle := oracleSession(t)
	root := tc.Root
	writeFile(t, root, "src/keep.txt", "x\n")
	tc.Cwd = filepath.Join(root, ".infuse")
	out, isErr := run(t, tc, "Bash", map[string]any{"command": "cd ../src\ncat > oracle/ready.md <<'E'\nhack\nE\necho done"})
	if isErr {
		t.Fatalf("%q", out)
	}
	if b, _ := os.ReadFile(oracle); string(b) != "ORIGINAL\n" {
		t.Fatal("the oracle was written through the transform")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "src", "oracle", "ready.md")); string(b) != "hack\n" {
		t.Fatalf("not written where the guard judged it: %q", b)
	}
}

func TestHeredocOutsideTheWorkRootIsRefused(t *testing.T) {
	s, tc, oracle := oracleSession(t)
	root := tc.Root
	before := blockedCount(root)
	if out, isErr := run(t, tc, "Bash", map[string]any{"command": "cat > ../evil.txt <<'E'\nhack\nE\necho ran"}); !isErr || strings.Contains(out, "ran") {
		t.Fatalf("%v %q", isErr, out)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil.txt")); err == nil {
		t.Fatal("the transform wrote outside the project")
	}
	if blockedCount(root) != before+1 {
		t.Fatal("no blocked entry")
	}
	in := shellWriteIntent{Abs: filepath.Join(filepath.Dir(root), "evil.txt"), Content: "x\n"}
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); why.ID != "path.outside" {
		t.Fatalf("the policy skipped a path outside the root: %q", why.Render("en"))
	}
	_ = oracle
}

func TestHeredocIsJudgedInTheDirectoryTheShellIsIn(t *testing.T) {
	s, tc, _ := oracleSession(t)
	s.setBans([]Ban{{Kind: "path", Value: "src/legacy", Quote: "do not touch src/legacy", Turn: 1}})
	writeFile(t, tc.Root, "src/legacy/keep.js", "export const x = 1;\n")
	if _, isErr := run(t, tc, "Bash", map[string]any{"command": "cd src\ncat > legacy/x.js <<'E'\nexport const y = 2;\nE"}); !isErr {
		t.Fatal("the ban was judged in the starting directory")
	}
}

func TestHeredocCheckIncludesTheInstructions(t *testing.T) {
	s, root := shellProject(t)
	in := shellWriteIntent{Abs: filepath.Join(root, "CLAUDE.md"), Content: "new rules\n"}
	writeFile(t, root, "CLAUDE.md", "the rules\n")
	if why := s.precheckShellWrites(context.Background(), []shellWriteIntent{in}); why.Empty() {
		t.Fatal("a heredoc rewrote the project's instructions")
	}
}

// `disable` used to stop the rewrite after the transform had already written the file.
func TestDisabledHeredocWritesNothingOnTheHost(t *testing.T) {
	_, tc, _ := oracleSession(t)
	tc.Rules = &RuleSet{Disabled: map[string]bool{"heredoc": true}}
	input, _ := json.Marshal(bashInput{Command: "cat > host.txt <<'E'\nx\nE"})
	var in bashInput
	_ = json.Unmarshal(input, &in)
	if _, _, _, _, msg, ok := prepareBash(context.Background(), tc, &in); !ok {
		t.Fatal(msg)
	}
	if _, err := os.Stat(filepath.Join(tc.Root, "host.txt")); err == nil {
		t.Fatal("the disabled transform wrote on the host")
	}
}

func TestRuleRewriteIsGuarded(t *testing.T) {
	_, tc, oracle := oracleSession(t)
	rep := "rm .infuse/oracle/ready.md"
	tc.Rules = &RuleSet{Disabled: map[string]bool{}, Rules: []Rule{{ID: "r", When: "command", Match: `^echo safe$`, Replace: &rep}}}
	if out, isErr := run(t, tc, "Bash", map[string]any{"command": "echo safe"}); !isErr {
		t.Fatalf("%q", out)
	}
	if b, _ := os.ReadFile(oracle); string(b) != "ORIGINAL\n" {
		t.Fatal("a rule rewrote a command into a write to the oracle")
	}
}

// Every refused attempt is journaled: the diagnosis gate used to stop the call before the path gate.
func TestProtectedWriteIsJournaledBeforeAnyGate(t *testing.T) {
	s, tc, oracle := oracleSession(t)
	before := blockedCount(tc.Root)
	input, _ := json.Marshal(map[string]any{"path": ".infuse/oracle/ready.md", "content": "hack"})
	out, isErr := s.runTool(context.Background(), tc.Rules, Block{ID: "c1", Name: "Write", Input: input})
	if !isErr || !strings.Contains(out, "protected path") {
		t.Fatalf("%v %q", isErr, out)
	}
	if blockedCount(tc.Root) != before+1 {
		t.Fatal("no blocked entry for the attempt")
	}
	if b, _ := os.ReadFile(oracle); string(b) != "ORIGINAL\n" {
		t.Fatal("the oracle changed")
	}
}

func TestUploadStaysOnReadableProjectFiles(t *testing.T) {
	_, tc, _ := oracleSession(t)
	root := tc.Root
	writeFile(t, root, "public/logo.png", "x")
	writeFile(t, root, ".infuse/sessions/s.json", "secret")
	if _, err := tc.ProjectFile("public/logo.png"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".infuse/sessions/s.json", "../x.png", filepath.Join(t.TempDir(), "x.png")} {
		if _, err := tc.ProjectFile(p); err == nil {
			t.Errorf("%s can be uploaded", p)
		}
	}
	outside := filepath.Join(t.TempDir(), "x.png")
	_ = os.WriteFile(outside, []byte("x"), 0o644)
	if err := os.Symlink(outside, filepath.Join(root, "public", "link.png")); err == nil {
		if _, err := tc.ProjectFile("public/link.png"); err == nil {
			t.Fatal("a link took an upload out of the project")
		}
	}
}
