//go:build windows

package sandbox

// The contract the Windows sandbox has to keep, whichever mechanism delivers it.

import (
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type sandboxFixture struct {
	root, outside, inside, existing, nested, protected string
}

func newSandboxFixture(t *testing.T) *sandboxFixture {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	f := &sandboxFixture{root: t.TempDir(), outside: filepath.Join(t.TempDir(), "outside.txt")}
	f.inside = filepath.Join(f.root, "inside.txt")
	f.existing = filepath.Join(f.root, "existing.txt")
	f.nested = filepath.Join(f.root, "sub", "nested.txt")
	f.protected = filepath.Join(f.root, ".infuse", "oracle.txt")
	mustWriteFile(t, f.outside, "original")
	mustWriteFile(t, f.existing, "before")
	mustWriteFile(t, f.protected, "criteria")
	return f
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Git Bash writes inside the work root: new files, existing ones and new folders.
func TestWindowsSandboxWritesInside(t *testing.T) {
	f := newSandboxFixture(t)
	slash := filepath.ToSlash
	script := "printf inside > '" + slash(f.inside) + "'; printf changed > '" + slash(f.existing) +
		"'; mkdir -p '" + slash(filepath.Dir(f.nested)) + "' && printf nested > '" + slash(f.nested) + "'"
	if code := bashRun(t, f.root, script); code != 0 {
		t.Fatalf("inside write failed: code=%d", code)
	}
	for path, want := range map[string]string{f.inside: "inside", f.existing: "changed", f.nested: "nested"} {
		if b, _ := os.ReadFile(path); string(b) != want {
			t.Fatalf("%s holds %q, want %q", path, b, want)
		}
	}
}

// Writes and deletes outside the work root are denied while reads stay allowed.
func TestWindowsSandboxBlocksOutsideWrites(t *testing.T) {
	f := newSandboxFixture(t)
	outside := filepath.ToSlash(f.outside)
	if code := bashRun(t, f.root, "printf hacked > '"+outside+"'"); code == 0 {
		t.Fatal("a write outside the work root succeeded")
	}
	if b, _ := os.ReadFile(f.outside); string(b) != "original" {
		t.Fatalf("outside file changed to %q", b)
	}
	if code := bashRun(t, f.root, "rm -f '"+outside+"'"); code == 0 {
		t.Fatal("a delete outside the work root succeeded")
	}
	if _, err := os.Stat(f.outside); err != nil {
		t.Fatal("the outside file was deleted")
	}
	if code := bashRun(t, f.root, "grep -q original '"+outside+"'"); code != 0 {
		t.Fatalf("reading outside must stay allowed: code=%d", code)
	}
}

// .infuse stays out of reach of the contained shell even though it lives inside the work root.
func TestWindowsSandboxProtectsInfuseRecords(t *testing.T) {
	f := newSandboxFixture(t)
	if code := bashRun(t, f.root, "printf changed > '"+filepath.ToSlash(f.protected)+"'"); code == 0 {
		t.Fatal("the protected .infuse record was writable")
	}
	if b, _ := os.ReadFile(f.protected); string(b) != "criteria" {
		t.Fatalf("the protected record changed to %q", b)
	}
}

// A glob in the user's protected paths reaches the kernel as the files it matches.
func TestWindowsSandboxProtectsGlobs(t *testing.T) {
	f := newSandboxFixture(t)
	secret := filepath.Join(f.root, "config", "prod.secret")
	mustWriteFile(t, secret, "kept")
	bash := core.FindRealBash()
	if bash == "" {
		t.Skip("Git Bash is not installed")
	}
	if code := sandboxRunWith(t, f.root, []string{"**/*.secret"}, bash, "-c", "printf x > config/prod.secret"); code == 0 {
		t.Fatal("a file matching a protected glob was writable")
	}
	if b, _ := os.ReadFile(secret); string(b) != "kept" {
		t.Fatalf("the protected file changed to %q", b)
	}
}

// A PATH inherited from Git Bash is POSIX-style and is converted before MSYS2 sees it.
func TestNormalizeMixedPath(t *testing.T) {
	got := normalizePath("/usr/bin:/c/Users/Jonathan/go/bin:C:/Tools/node")
	want := `C:\Users\Jonathan\go\bin;C:\Tools\node`
	if got != want {
		t.Fatalf("normalizePath = %q, want %q", got, want)
	}
	win := normalizePath(`C:\Windows;C:/Tools/node`)
	if win != `C:\Windows;C:\Tools\node` {
		t.Fatalf("windows PATH changed: %q", win)
	}
}

func TestWrapUsesTheHiddenWrapper(t *testing.T) {
	t.Setenv("INFUSE_NO_SANDBOX", "")
	argv := Wrap([]string{"bash.exe", "-c", "echo hi"}, `C:\work`, []string{".env"}, nil)
	if len(argv) < 6 || argv[1] != Role || argv[3] != `C:\work` {
		t.Fatalf("unexpected wrapper argv: %#v", argv)
	}
	if argv[len(argv)-2] != "-c" || argv[len(argv)-1] != "echo hi" {
		t.Fatalf("the command was not preserved: %#v", argv)
	}
}

// The production path prepares a private MSYS2 runtime once and reuses it, so the contained shell has
// its own namespace instead of colliding with the user's Git Bash.
func TestWindowsSandboxBuildsPrivateMSYS(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	bash := core.FindRealBash()
	if bash == "" {
		t.Skip("Git Bash is not installed")
	}
	argv, _, err := containedShell([]string{bash, "-c", "true"})
	if err != nil {
		t.Fatal(err)
	}
	private := argv[0]
	if strings.EqualFold(private, bash) {
		t.Fatalf("expected a private runtime, got %q", private)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(private), "msys-2.0.dll")); err != nil {
		t.Fatalf("the private runtime has no msys-2.0.dll: %v", err)
	}
	again, _, err := containedShell([]string{bash, "-c", "true"})
	if err != nil || again[0] != private {
		t.Fatalf("the private runtime was not reused: %q err=%v", again[0], err)
	}
	root := t.TempDir()
	if code := bashRun(t, root, "printf ready > ready.txt"); code != 0 {
		t.Fatalf("private runtime command failed: code=%d", code)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "ready.txt")); string(b) != "ready" {
		t.Fatalf("private runtime wrote %q", b)
	}
}

// Runtimes of earlier Git versions go; the current one, and one being built, stay.
func TestOldRuntimesArePruned(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"current", "old", "current.123"} {
		if err := os.Mkdir(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pruneOldRuntimes(dir, "current")
	if _, err := os.Stat(filepath.Join(dir, "old")); err == nil {
		t.Fatal("an old runtime stayed")
	}
	for _, name := range []string{"current", "current.123"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was removed", name)
		}
	}
}

// The wrapper fails closed when the work root is unusable, instead of running without the sandbox.
func TestWindowsSandboxRefusesBadRoot(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	wrapped := Wrap([]string{"cmd", "/c", "exit 0"}, missing, nil, nil)
	cmd := exec.Command(exe, wrapped[1:]...)
	cmd.Env = append(os.Environ(), "INFUSE_SANDBOX_HELPER=1")
	if err := cmd.Run(); err == nil {
		t.Fatal("a missing work root must fail closed")
	}
}
