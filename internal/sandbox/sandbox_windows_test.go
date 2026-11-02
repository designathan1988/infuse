//go:build windows

package sandbox

import (
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// bash runs a script in the contained Git Bash the agent's Bash tool uses.
func bashRun(t *testing.T, root, script string) int {
	t.Helper()
	bash := core.FindRealBash()
	if bash == "" {
		t.Skip("Git Bash is not installed")
	}
	return sandboxRun(t, root, bash, "-c", script)
}

// TestContainedBashStartsWhileAMediumBashIsAlive: a user's open Git Bash owns the MSYS namespace at
// Medium, which a Low bash cannot map; the private runtime exists for this.
func TestContainedBashStartsWhileAMediumBashIsAlive(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	bash := core.FindRealBash()
	if bash == "" {
		t.Skip("Git Bash is not installed")
	}
	medium := exec.Command(bash, "-c", "sleep 30")
	if err := medium.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = medium.Process.Kill(); _ = medium.Wait() }()
	if code := bashRun(t, root, "echo inside > out.txt && ls out.txt >/dev/null"); code != 0 {
		t.Fatalf("contained bash failed: exit %d", code)
	}
}

func TestBashStatusAndToolsSurviveContainment(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if code := bashRun(t, root, "exit 7"); code != 7 {
		t.Fatalf("exit status lost: %d", code)
	}
	if code := bashRun(t, root, "mkdir -p a/b && echo x > a/b/f && mv a/b/f a/g && rm -rf a && echo y > \"$TMP/t\""); code != 0 {
		t.Fatalf("ordinary work in the project and in temp failed: exit %d", code)
	}
}

// The parent's delete-child right must not open a way around a Medium child.
func TestProtectedPathsCannotBeRenamedOrDeleted(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755)
	mcp := filepath.Join(root, ".mcp.json")
	if err := os.WriteFile(mcp, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	protected := []string{".infuse/oracle", ".mcp.json"}
	for _, script := range []string{"mv .infuse moved", "rm -f .mcp.json", "mv .mcp.json x", "rm -rf .infuse"} {
		bash := core.FindRealBash()
		if bash == "" {
			t.Skip("Git Bash is not installed")
		}
		_ = sandboxRunWith(t, root, protected, bash, "-c", script)
		if _, err := os.Stat(filepath.Join(root, ".infuse", "oracle")); err != nil {
			t.Fatalf("%s: .infuse is gone: %v", script, err)
		}
		if _, err := os.Stat(mcp); err != nil {
			t.Fatalf("%s: .mcp.json is gone: %v", script, err)
		}
	}
}

func TestGitHooksAndConfigAreOutOfReach(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", ".")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git: %s", out)
	}
	config := filepath.Join(root, ".git", "config")
	before, _ := os.ReadFile(config)
	_ = bashRun(t, root, "echo evil >> .git/config; echo evil > .git/hooks/post-checkout")
	after, _ := os.ReadFile(config)
	if string(before) != string(after) {
		t.Fatal(".git/config changed: a host-side git would run what it says")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "post-checkout")); err == nil {
		t.Fatal("a hook was planted")
	}
	if code := bashRun(t, root, "echo x > f && git add f"); code != 0 {
		t.Fatalf("git must keep working in the project: exit %d", code)
	}
}

func TestLabelTreeIsIdempotent(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".infuse"), 0o755)
	for range 2 {
		if err := labelTree(root, nil); err != nil {
			t.Fatal(err)
		}
	}
	out, err := exec.Command("icacls", filepath.Join(root, ".infuse")).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(out), "(NW)"); n != 1 {
		t.Fatalf("expected one label on .infuse, got %d:\n%s", n, out)
	}
}
