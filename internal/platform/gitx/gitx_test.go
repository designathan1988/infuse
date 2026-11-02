package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		if out, err := Run(root, args...); err != nil {
			t.Fatal(out)
		}
	}
	return root
}

func gitConfig(t *testing.T, root, key, value string) {
	t.Helper()
	cmd := exec.Command("git", "config", key, value)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
}

// plant writes a script that leaves a marker when something runs it and returns the marker and the
// script's path, in the form git hands to its shell.
func plant(t *testing.T, root, name string) (marker, script string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), name+".ran")
	p := filepath.Join(root, ".planted-"+name+".sh")
	body := "#!/bin/sh\necho ran > '" + filepath.ToSlash(marker) + "'\ncat\n"
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return marker, filepath.ToSlash(p)
}

func assertNotRun(t *testing.T, marker, what string) {
	t.Helper()
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("%s ran a program named by the repository", what)
	}
}

func TestHooksDoNotRun(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "hook")
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nsh '"+script+"' </dev/null\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644)
	if out, err := Run(root, "add", "a.txt"); err != nil {
		t.Fatal(out)
	}
	if out, err := Run(root, "commit", "-qm", "x"); err != nil {
		t.Fatal(out)
	}
	assertNotRun(t, marker, "git commit")
}

func TestFsmonitorDoesNotRun(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "fsmonitor")
	gitConfig(t, root, "core.fsmonitor", script)
	_ = Output(root, "status", "--porcelain")
	assertNotRun(t, marker, "git status")
}

func TestCleanFilterDoesNotRun(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "clean")
	gitConfig(t, root, "filter.evil.clean", "sh "+script)
	gitConfig(t, root, "filter.evil.smudge", "sh "+script)
	_ = os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("*.txt filter=evil\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644)
	if out, err := Run(root, "add", "-A"); err != nil {
		t.Fatal(out)
	}
	assertNotRun(t, marker, "git add")
}

func TestDiffDriverDoesNotRun(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "textconv")
	gitConfig(t, root, "diff.evil.textconv", "sh "+script)
	_ = os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("*.txt diff=evil\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644)
	_, _ = Run(root, "add", "-A")
	_, _ = Run(root, "commit", "-qm", "x")
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("b\n"), 0o644)
	if out, err := Run(root, "diff"); err != nil {
		t.Fatal(out)
	}
	assertNotRun(t, marker, "git diff")
}

func TestExternalDiffDoesNotRun(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "external")
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("a\n"), 0o644)
	_, _ = Run(root, "add", "-A")
	_, _ = Run(root, "commit", "-qm", "x")
	gitConfig(t, root, "diff.external", script)
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("b\n"), 0o644)
	if out, err := Run(root, "diff"); err != nil {
		t.Fatal(out)
	}
	assertNotRun(t, marker, "git diff")
}

func TestExtProtocolIsRefused(t *testing.T) {
	root := repo(t)
	marker, script := plant(t, root, "ext")
	out, err := Run(root, "ls-remote", "ext::sh "+script)
	if err == nil {
		t.Fatalf("the ext transport must be refused: %s", out)
	}
	assertNotRun(t, marker, "git ls-remote")
}

func TestRepositoryCredentialHelperIsCleared(t *testing.T) {
	args := strings.Join(Overrides(t.TempDir()), " ")
	if !strings.Contains(args, "credential.helper= ") && !strings.HasSuffix(args, "credential.helper=") {
		t.Fatalf("the helper list must be reset first: %s", args)
	}
}

func TestEnvCarriesTheOverrides(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Setenv("GIT_CONFIG_COUNT", "")
	}
	root := repo(t)
	marker, script := plant(t, root, "envhook")
	gitConfig(t, root, "core.fsmonitor", script)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	cmd.Env = Env()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	assertNotRun(t, marker, "a git started by another program")
}
