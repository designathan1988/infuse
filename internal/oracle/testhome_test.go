package oracle

import (
	"infuse/internal/filter"
	"infuse/internal/workspace"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

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
		if out, err := workspace.GitRun(root, args...); err != nil {
			t.Fatal(out)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644)
	_, _ = workspace.GitRun(root, "add", "-A")
	if out, err := workspace.GitRun(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(out)
	}
	return root
}

func gitOutput(root string, args ...string) string { return workspace.GitOutput(root, args...) }

func bypassRoot(t *testing.T) (string, *filter.Guard) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	oracle := filepath.Join(root, ".infuse", "oracle", "pronto.md")
	_ = os.MkdirAll(filepath.Dir(oracle), 0o755)
	_ = os.WriteFile(oracle, []byte("ORIGINAL\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "secrets"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "secrets", "key.pem"), []byte("K\n"), 0o644)
	return root, filter.NewGuard(root, filter.Policy{Protected: []string{"secrets/**"}})
}
