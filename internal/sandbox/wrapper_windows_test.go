//go:build windows

package sandbox

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sandboxBash(t *testing.T) string {
	t.Helper()
	raw, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Git Bash is not available")
	}
	private, err := prepareMSYS(raw)
	if err != nil || private == "" {
		t.Fatalf("prepare the private MSYS2 runtime: %v", err)
	}
	return private
}

type sandboxFixture struct {
	root, outside, inside, existing, nested, protected, bash string
}

func newSandboxFixture(t *testing.T) *sandboxFixture {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	f := &sandboxFixture{
		root:    t.TempDir(),
		outside: filepath.Join(t.TempDir(), "outside.txt"),
		bash:    sandboxBash(t),
	}
	f.inside = filepath.Join(f.root, "inside.txt")
	f.existing = filepath.Join(f.root, "existing.txt")
	f.nested = filepath.Join(f.root, "sub", "nested.txt")
	f.protected = filepath.Join(f.root, ".infuse", "oracle.txt")
	mustWrite(t, f.outside, "original")
	mustWrite(t, f.existing, "before")
	mustWrite(t, f.protected, "criteria")
	return f
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *sandboxFixture) run(t *testing.T, command string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code, err := runConfined(f.root, nil, []string{f.bash, "-c", command}, filepath.Dir(f.bash), strings.NewReader(""), &out, &out)
	if err != nil {
		t.Fatalf("sandbox setup failed: %v", err)
	}
	return code, out.String()
}

// Git Bash keeps the caller's reads and writes inside the work root.
func TestWindowsSandboxWritesInside(t *testing.T) {
	f := newSandboxFixture(t)
	slash := filepath.ToSlash
	command := "printf inside > '" + slash(f.inside) + "'; printf changed > '" + slash(f.existing) +
		"'; mkdir -p '" + slash(filepath.Dir(f.nested)) + "' && printf nested > '" + slash(f.nested) + "'"
	code, out := f.run(t, command)
	if code != 0 {
		t.Fatalf("inside write failed: code=%d out=%s", code, out)
	}
	for path, want := range map[string]string{f.inside: "inside", f.existing: "changed", f.nested: "nested"} {
		if b, _ := os.ReadFile(path); string(b) != want {
			t.Fatalf("%s holds %q, want %q", path, b, want)
		}
	}
	if !grantIsStanding(f.root, capabilitySID(f.root).sid) {
		t.Fatal("the standing grant marker was not written")
	}
}

// Writes and deletes outside the work root are denied while reads stay allowed.
func TestWindowsSandboxBlocksOutsideWrites(t *testing.T) {
	f := newSandboxFixture(t)
	outside := filepath.ToSlash(f.outside)
	if code, out := f.run(t, "printf hacked > '"+outside+"'"); code == 0 {
		t.Fatalf("a write outside the work root succeeded: %s", out)
	}
	if b, _ := os.ReadFile(f.outside); string(b) != "original" {
		t.Fatalf("outside file changed to %q", b)
	}
	if code, _ := f.run(t, "rm -f '"+outside+"'"); code == 0 {
		t.Fatal("a delete outside the work root succeeded")
	}
	if _, err := os.Stat(f.outside); err != nil {
		t.Fatal("the outside file was deleted")
	}
	code, out := f.run(t, "cat '"+outside+"'")
	if code != 0 || !strings.Contains(out, "original") {
		t.Fatalf("reading outside must stay allowed: code=%d out=%s", code, out)
	}
}

// .infuse stays out of reach of the confined shell even though it lives inside the work root.
func TestWindowsSandboxProtectsInfuseRecords(t *testing.T) {
	f := newSandboxFixture(t)
	if code, _ := f.run(t, "printf changed > '"+filepath.ToSlash(f.protected)+"'"); code == 0 {
		t.Fatal("the protected .infuse record was writable")
	}
	if b, _ := os.ReadFile(f.protected); string(b) != "criteria" {
		t.Fatalf("the protected record changed to %q", b)
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
	t.Setenv("INFUSE_SANDBOX_FORCE", "1")
	argv := Wrap([]string{"bash.exe", "-c", "echo hi"}, `C:\work`, []string{".env"})
	if len(argv) < 6 || argv[1] != "__sandbox" || argv[2] != `C:\work` || argv[4] != "--" {
		t.Fatalf("unexpected wrapper argv: %#v", argv)
	}
	if argv[len(argv)-2] != "-c" || argv[len(argv)-1] != "echo hi" {
		t.Fatalf("the command was not preserved: %#v", argv)
	}
}

// The production path prepares a private MSYS2 runtime once and reuses it, so the confined shell has
// its own namespace instead of colliding with the user's Git Bash.
func TestWindowsSandboxBuildsPrivateMSYS(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("Git Bash is not available")
	}
	private, err := prepareMSYS(bash)
	if err != nil {
		t.Fatal(err)
	}
	if private == bash || private == "" {
		t.Fatalf("expected a private runtime, got %q", private)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(private), "msys-2.0.dll")); err != nil {
		t.Fatalf("the private runtime has no msys-2.0.dll: %v", err)
	}
	again, err := prepareMSYS(bash)
	if err != nil || again != private {
		t.Fatalf("the private runtime was not reused: %q err=%v", again, err)
	}
	root := t.TempDir()
	var out bytes.Buffer
	ready := filepath.ToSlash(filepath.Join(root, "ready.txt"))
	code, err := runConfined(root, nil, []string{private, "-c", "printf ready > '" + ready + "'"}, filepath.Dir(private), strings.NewReader(""), &out, &out)
	if err != nil || code != 0 {
		t.Fatalf("private runtime command failed: code=%d err=%v out=%s", code, err, out.String())
	}
	if b, _ := os.ReadFile(filepath.Join(root, "ready.txt")); string(b) != "ready" {
		t.Fatalf("private runtime wrote %q", b)
	}
}

// The wrapper fails closed when the token or the ACL cannot be applied, instead of running without
// the sandbox.
func TestWindowsSandboxRefusesBadRoot(t *testing.T) {
	if code, err := runConfined(filepath.Join(t.TempDir(), "missing"), nil, []string{"cmd", "/c", "exit 0"}, "", nil, nil, nil); err == nil || code == 0 {
		t.Fatalf("a missing work root must fail closed: code=%d err=%v", code, err)
	}
}
