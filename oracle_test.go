package main

// The oracle is unreachable for the agent through every tool: Write, Edit, Bash heredoc, Bash
// redirection, Bash rm. Each attempt fails, is journaled as "blocked", and the file is intact afterwards.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func oracleSession(t *testing.T) (*Session, *ToolContext, string) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	oracle := filepath.Join(root, ".infuse", "oracle", "pronto.md")
	_ = os.MkdirAll(filepath.Dir(oracle), 0o755)
	_ = os.WriteFile(oracle, []byte("ORIGINAL\n"), 0o644)
	s := newSession(root)
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: LoadRules(root)}
	return s, tc, oracle
}

func run(t *testing.T, tc *ToolContext, tool string, input any) (string, bool) {
	t.Helper()
	b, _ := json.Marshal(input)
	return toolRegistry[tool].Run(context.Background(), tc, b)
}

func blockedCount(root string) int {
	n := 0
	for _, e := range readJSONL(filepath.Join(root, infuseDirName, "journal.jsonl")) {
		if e["kind"] == "blocked" {
			n++
		}
	}
	return n
}

func TestOracleUnreachableByEveryTool(t *testing.T) {
	s, tc, oracle := oracleSession(t)
	rel := ".infuse/oracle/pronto.md"

	if out, isErr := run(t, tc, "Write", map[string]any{"path": rel, "content": "hack"}); !isErr || !strings.Contains(out, "protegido") {
		t.Fatalf("Write: %v %q", isErr, out)
	}
	s.markRead(oracle)
	if _, isErr := run(t, tc, "Edit", map[string]any{"path": rel, "old_string": "ORIGINAL", "new_string": "hack"}); !isErr {
		t.Fatal("Edit must fail")
	}
	if out, isErr := run(t, tc, "Bash", map[string]any{"command": "cat > .infuse/oracle/pronto.md <<'EOF'\nhack\nEOF\necho done"}); !isErr || strings.Contains(out, "done") {
		t.Fatalf("heredoc: %v %q", isErr, out)
	}
	if out, _ := run(t, tc, "Bash", map[string]any{"command": "echo hack > .infuse/oracle/pronto.md; echo hack2 >> " + strings.ReplaceAll(oracle, `\`, "/") + "; echo ran"}); !strings.Contains(out, "ran") || !strings.Contains(out, "neutralized") {
		t.Fatalf("redirect: %q", out)
	}
	if _, isErr := run(t, tc, "Bash", map[string]any{"command": "rm -f .infuse/oracle/pronto.md"}); !isErr {
		t.Fatal("rm must be refused")
	}
	if _, isErr := run(t, tc, "Bash", map[string]any{"command": "mv .infuse/oracle/pronto.md /tmp/x && echo ok"}); !isErr {
		t.Fatal("mv must be refused")
	}
	if _, isErr := run(t, tc, "Bash", map[string]any{"command": "tee .infuse/oracle/pronto.md < /dev/null"}); !isErr {
		t.Fatal("tee must be refused")
	}

	b, err := os.ReadFile(oracle)
	if err != nil || string(b) != "ORIGINAL\n" {
		t.Fatalf("oracle changed: %q %v", b, err)
	}
	if n := blockedCount(tc.Root); n < 7 {
		t.Fatalf("expected every attempt journaled as blocked, got %d", n)
	}
	// and nothing was checkpointed for the refused heredoc (no write happened)
	if len(s.Checkpoints[s.Turn]) != 0 {
		t.Fatalf("checkpoint recorded for a refused write: %+v", s.Checkpoints)
	}
}

func TestGuardLeavesOrdinaryCommandsAlone(t *testing.T) {
	root := t.TempDir()
	g := newGuard(root, []string{"secrets/**"})
	for _, c := range []string{"ls -la", "echo hi > out.txt", "cat a.txt | grep x", "rm -rf node_modules", "git status 2>&1", "mv a b && echo ok"} {
		out, notes, err := g.GuardCommand(c, root)
		if err != nil || out != c || len(notes) != 0 {
			t.Fatalf("%q changed: %q %v %v", c, out, notes, err)
		}
	}
	if _, _, err := g.GuardCommand("rm secrets/key.pem", root); err == nil {
		t.Fatal("user-protected glob not enforced")
	}
	out, notes, err := g.GuardCommand(`echo x >secrets/a && echo y 2>> "secrets/b"`, root)
	if err != nil || len(notes) != 2 || strings.Count(out, "/dev/null") != 2 {
		t.Fatalf("%q %v %v", out, notes, err)
	}
	if _, _, err := g.GuardCommand("cd secrets && rm key.pem", filepath.Join(root, "x")); err == nil {
		// relative to cwd=x: secrets/key.pem → x/secrets/key.pem is NOT protected; `cd` is not followed.
		// This is documented: protection is by resolved path from the tool's cwd.
	}
	if _, _, err := g.GuardCommand("rm "+filepath.Join(root, ".infuse", "oracle", "x.md"), filepath.Join(root, "sub")); err == nil {
		t.Fatal("absolute oracle path not caught")
	}
}
