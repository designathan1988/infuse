package workspace

import (
	"infuse/internal/sandbox"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain plays the __sandbox role: contained commands re-execute this binary as the real one would.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == sandbox.Role {
		os.Exit(sandbox.Main(os.Args[2:]))
	}
	os.Exit(m.Run())
}

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		if out, err := GitRun(root, args...); err != nil {
			t.Fatal(out)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644)
	_, _ = GitRun(root, "add", "-A")
	if out, err := GitRun(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(out)
	}
	return root
}
