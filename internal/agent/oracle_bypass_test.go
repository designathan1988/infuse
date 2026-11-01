package agent

// The guard's bypass battery: every route an agent could use to reach a protected path through
// Bash. Each one must be refused (or neutralized, for plain redirections) and journaled, with the
// protected content intact at the end.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func bypassRoot(t *testing.T) (string, *Guard) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	oracle := filepath.Join(root, ".infuse", "oracle", "ready.md")
	_ = os.MkdirAll(filepath.Dir(oracle), 0o755)
	_ = os.WriteFile(oracle, []byte("ORIGINAL\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "secrets"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "secrets", "key.pem"), []byte("K\n"), 0o644)
	return root, newGuard(root, []string{"secrets/**"})
}

func mustRefuse(t *testing.T, g *Guard, cwd, cmd string) {
	t.Helper()
	_, _, err := g.GuardCommand(cmd, cwd)
	if err == nil {
		t.Fatalf("bypass accepted: %q", cmd)
	}
}

func TestGuardCdBypassClosed(t *testing.T) {
	root, g := bypassRoot(t)
	mustRefuse(t, g, root, "cd .infuse/oracle && rm ready.md")
	mustRefuse(t, g, root, "cd .infuse/oracle && cat > ready.md <<'EOF'\nhack\nEOF")
	mustRefuse(t, g, root, "cd .infuse/oracle; mv ready.md /tmp/x")
	mustRefuse(t, g, root, "cd .infuse/oracle && tee ready.md </dev/null")
	mustRefuse(t, g, root, "cd secrets && rm key.pem")
	// redirection after cd is neutralized (same semantics as a top-level redirection)
	out, notes, err := g.GuardCommand("cd .infuse/oracle && echo hacked > ready.md && echo ok", root)
	if err != nil {
		t.Fatalf("redirection must be neutralized, not refused: %v", err)
	}
	if !strings.Contains(out, "/dev/null") || !strings.Contains(out, "echo ok") || len(notes) == 0 {
		t.Fatalf("wrong post-cd neutralization: %q %v", out, notes)
	}
	// popd restores the directory
	mustRefuse(t, g, root, "pushd secrets && popd && cd secrets && rm key.pem")
	// an ordinary cd must not break anything
	if out, _, err := g.GuardCommand("cd src && go build ./... && cd ..", root); err != nil || !strings.Contains(out, "go build") {
		t.Fatalf("ordinary command changed: %q %v", out, err)
	}
}

func TestGuardFindXargsNestedClosed(t *testing.T) {
	root, g := bypassRoot(t)
	mustRefuse(t, g, root, "find .infuse/oracle -name '*.md' -delete")
	mustRefuse(t, g, root, "find . -type f -exec rm {} .infuse/oracle/ready.md \\;")
	mustRefuse(t, g, root, "printf '%s\\n' .infuse/oracle/ready.md | xargs rm")
	mustRefuse(t, g, root, "ls secrets | xargs rm -f")
	// nested shells: payload inspected, refuse on any protected involvement
	mustRefuse(t, g, root, `cmd /c "del .infuse\oracle\ready.md"`)
	mustRefuse(t, g, root, `bash -c "cd .infuse/oracle; rm ready.md"`)
	mustRefuse(t, g, root, `sh -c 'echo x > .infuse/oracle/ready.md'`)
	mustRefuse(t, g, root, `bash -c "echo hack > .infuse/oracle/ready.md"`) // nested redirect = refuse, not neutralize
	// nested shells doing ordinary things keep working
	if out, _, err := g.GuardCommand(`bash -c "echo hi > normal.txt"`, root); err != nil || !strings.Contains(out, "normal.txt") {
		t.Fatalf("ordinary nested shell refused: %q %v", out, err)
	}
	// git subcommands that destroy
	mustRefuse(t, g, root, "git rm .infuse/oracle/ready.md")
	mustRefuse(t, g, root, "git stash push -- .infuse/oracle/ready.md")
	mustRefuse(t, g, root, "git clean -f .infuse/oracle")
	// ordinary git untouched
	if out, _, err := g.GuardCommand("git status --porcelain", root); err != nil || strings.Contains(out, "/dev/null") {
		t.Fatalf("ordinary git changed: %q %v", out, err)
	}
	// find that only reads is untouched
	if _, _, err := g.GuardCommand("find .infuse/oracle -name '*.md'", root); err != nil {
		t.Fatalf("read-only find refused: %v", err)
	}
}

func TestGuardSymlinkClosed(t *testing.T) {
	root, g := bypassRoot(t)
	link := filepath.Join(root, "link.md")
	if err := os.Symlink(filepath.Join(root, ".infuse", "oracle", "ready.md"), link); err != nil {
		t.Skipf("no permission for a symlink on this machine: %v", err)
	}
	out, notes, err := g.GuardCommand("echo hack > link.md", root)
	if err != nil {
		t.Fatalf("symlink must be neutralized: %v", err)
	}
	if !strings.Contains(out, "/dev/null") || len(notes) == 0 {
		t.Fatalf("redirect through the symlink passed: %q %v", out, notes)
	}
	mustRefuse(t, g, root, "rm link.md")
	b, e := os.ReadFile(filepath.Join(root, ".infuse", "oracle", "ready.md"))
	if e != nil || string(b) != "ORIGINAL\n" {
		t.Fatalf("oracle changed: %q %v", b, e)
	}
}

func TestGuardBypassesLeaveOracleIntact(t *testing.T) {
	root, g := bypassRoot(t)
	for _, cmd := range []string{
		"cd .infuse/oracle && rm ready.md",
		"find .infuse/oracle -delete",
		`bash -c "cd .infuse/oracle; rm ready.md"`,
		"git rm .infuse/oracle/ready.md",
	} {
		_, _, _ = g.GuardCommand(cmd, root)
	}
	b, err := os.ReadFile(filepath.Join(root, ".infuse", "oracle", "ready.md"))
	if err != nil || string(b) != "ORIGINAL\n" {
		t.Fatalf("oracle changed through some vector: %q %v", b, err)
	}
}

func TestGuardReadWriteRedirectClosed(t *testing.T) {
	root, g := bypassRoot(t)
	out, notes, err := g.GuardCommand("exec 3<> .infuse/oracle/ready.md; echo x >&3", root)
	if err != nil || !strings.Contains(out, "/dev/null") || len(notes) == 0 {
		t.Fatalf("<> not neutralized: %q %v %v", out, notes, err)
	}
}
