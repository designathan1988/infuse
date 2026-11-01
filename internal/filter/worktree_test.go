package filter

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGuardProtectsWorktreeOracle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-q", wt, "-b", "infuse-test")

	g := NewGuard(wt, Policy{})
	if _, ok := g.Protects(wt, ".infuse/oracle/ready.md"); !ok {
		t.Fatal("worktree oracle not protected")
	}
	if _, ok := g.Protects(wt, filepath.Join(root, ".infuse", "oracle", "x.md")); !ok {
		t.Fatal("main oracle reachable from a worktree")
	}
}
