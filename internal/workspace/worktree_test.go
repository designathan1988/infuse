package workspace

import (
	"infuse/internal/core"
	"infuse/internal/sandbox"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Creating a worktree checks files out: a hook the project planted must not run on the host.
func TestWorktreeFiresNoProjectHook(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	hook := "#!/bin/sh\necho ran > '" + filepath.ToSlash(marker) + "'\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "hooks", "post-checkout"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateWorktree(root, "x"); err != nil {
		t.Fatal(err)
	}
	if core.Exists(marker) {
		t.Fatal("a project hook ran on the host")
	}
}

// A dev server's command is approved, the code behind it is not: it runs contained.
func TestDevServerRunsContained(t *testing.T) {
	bash := core.FindRealBash()
	if bash == "" || !sandbox.Available() {
		t.Skip("no bash or no OS containment")
	}
	testHome(t)
	root := t.TempDir()
	escaped := filepath.Join(t.TempDir(), "escaped.txt")
	inside := filepath.Join(root, "inside.txt")
	script := "echo x > '" + filepath.ToSlash(escaped) + "'; echo x > '" + filepath.ToSlash(inside) + "'"
	cmd, err := devCommand(root, LaunchConfig{RuntimeExecutable: bash, RuntimeArgs: []string{"-c", script}})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Dir = root
	_ = cmd.Run()
	if core.Exists(escaped) {
		t.Fatal("a dev server wrote outside the project")
	}
	if !core.Exists(inside) {
		t.Fatal("the contained dev server did not run")
	}
}

func TestWorktreeAndChanges(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	dir, branch, err := CreateWorktree(root, "Minha Feature!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(branch, "infuse/minha-feature") || !core.Exists(filepath.Join(dir, "a.txt")) {
		t.Fatalf("%s %s", dir, branch)
	}
	if st := GitOutput(root, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Fatalf("worktree folder shows as a change in the project: %q", st)
	}
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\nTWO\nthree\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x\ny\n"), 0o644)
	c := Changes(dir)
	files := c["files"].([]FileChange)
	if len(files) != 2 || c["added"].(int) != 4 || c["removed"].(int) != 1 {
		t.Fatalf("%+v", c)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(b) != "one\ntwo\n" {
		t.Fatal("worktree edit leaked into the main checkout")
	}
	if err := RemoveWorktree(root, dir, true); err != nil {
		t.Fatal(err)
	}
}

func TestUserFileWriteConflict(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	m, err := WriteUserFile(root, "n.txt", "a", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(root, "n.txt"), []byte("disk"), 0o644)
	if _, err := WriteUserFile(root, "n.txt", "mine", m-1000, false); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := WriteUserFile(root, "n.txt", "mine", m-1000, true); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteUserFile(root, "../x.txt", "no", 0, true); err == nil {
		t.Fatal("write outside the project accepted")
	}
}
