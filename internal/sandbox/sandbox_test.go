package sandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain intercepts the helper invocations before the test framework starts: the test binary
// re-executes itself as the __sandbox role exactly the way the real binary is invoked, so the
// containment is exercised end to end, and no test runs inside a sandboxed helper.
func TestMain(m *testing.M) {
	if os.Getenv("INFUSE_SANDBOX_HELPER") == "1" {
		for i, a := range os.Args {
			if a == Role {
				os.Exit(Main(os.Args[i+1:]))
			}
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

// sandboxRun runs argv through the wrapper (via this test binary, as the real role runs) with cwd
// inside root, and returns its exit code.
func sandboxRun(t *testing.T, root string, argv ...string) int {
	t.Helper()
	return sandboxRunWith(t, root, []string{".infuse/oracle", ".infuse/evidence"}, argv...)
}

func sandboxRunWith(t *testing.T, root string, protected []string, argv ...string) int {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	wrapped := Wrap(argv, root, protected, nil)
	cmd := exec.Command(exe, wrapped[1:]...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "INFUSE_SANDBOX_HELPER=1")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	err = cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return 0
}

func shellArgv(script string) []string {
	if runtime.GOOS == "windows" {
		return []string{os.Getenv("ComSpec"), "/c", script}
	}
	return []string{"/bin/sh", "-c", script}
}

func TestSandboxAllowsProjectWritesAndReads(t *testing.T) {
	if !Available() {
		t.Skip("this platform has no OS containment")
	}
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if code := sandboxRun(t, root, shellArgv("echo worked > "+filepath.Join("out.txt"))...); code != 0 {
		t.Fatalf("a write inside the project failed: exit %d", code)
	}
	if b, err := os.ReadFile(filepath.Join(root, "out.txt")); err != nil || !strings.Contains(string(b), "worked") {
		t.Fatalf("%v %q", err, b)
	}
	// reading the machine is never restricted
	if code := sandboxRun(t, root, shellArgv(readFileCommand())...); code != 0 {
		t.Fatalf("reads must stay unrestricted: exit %d", code)
	}
}

// readFileCommand reads a file that always exists outside any project, on the current platform.
func readFileCommand() string {
	if runtime.GOOS == "windows" {
		return "type C:\\Windows\\win.ini"
	}
	return "cat /etc/hostname"
}

func TestSandboxRefusesWritesOutsideTheProject(t *testing.T) {
	if !Available() {
		t.Skip("this platform has no OS containment")
	}
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "escape.txt")
	script := "echo escaped > " + target
	if runtime.GOOS == "windows" {
		script = "echo escaped > " + strings.ReplaceAll(target, `\`, `\\`)
	}
	if code := sandboxRun(t, root, shellArgv(script)...); code == 0 {
		if b, err := os.ReadFile(target); err == nil {
			t.Fatalf("wrote outside the project: %q", b)
		}
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("the file outside the project exists")
	}
}

func TestSandboxRefusesWritesToInfuseRecords(t *testing.T) {
	if !Available() {
		t.Skip("this platform has no OS containment")
	}
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755)
	// overwriting an existing record and creating a new one inside .infuse both fail
	for _, script := range []string{
		"echo forged > " + filepath.Join(".infuse", "oracle", "checks.json"),
		"echo forged > " + filepath.Join(".infuse", "base.json"),
	} {
		if code := sandboxRun(t, root, shellArgv(script)...); code == 0 {
			t.Fatalf("wrote an infuse record: %s", script)
		}
	}
	// the project itself still accepts writes after the refusals
	if code := sandboxRun(t, root, shellArgv("echo ok > ok.txt")...); code != 0 {
		t.Fatalf("project write broken after refusals: exit %d", code)
	}
}

// The executor's children get their own caches and do not see infuse's variables.
func TestIsolatedWrapCarriesItsKey(t *testing.T) {
	if !Available() {
		t.Skip("this platform has no OS containment")
	}
	got := strings.Join(WrapIsolated([]string{"x"}, "root", nil, "secret", nil), " ")
	if !strings.Contains(got, "-k secret") {
		t.Fatalf("%s", got)
	}
	if strings.Contains(strings.Join(Wrap([]string{"x"}, "root", nil, nil), " "), " -k ") {
		t.Fatal("the agent's shell has no isolation key")
	}
}

func TestDropInfuseEnvKeepsTheHomeForTheWrapper(t *testing.T) {
	home := t.TempDir()
	t.Setenv("INFUSE_HOME", home)
	t.Setenv("INFUSE_ANYTHING", "1")
	t.Cleanup(func() { homeOverride = "" })
	dropInfuseEnv()
	if os.Getenv("INFUSE_ANYTHING") != "" || os.Getenv("INFUSE_HOME") != "" {
		t.Fatal("infuse's variables reached the child")
	}
	if got, err := infuseHome(); err != nil || got != home {
		t.Fatalf("%q %v", got, err)
	}
}

func TestWrapKeepsArgvWhenUnavailableOrOff(t *testing.T) {
	argv := []string{"/bin/bash", "-c", "echo hi"}
	off := false
	if got := Wrap(argv, ".", nil, &off); len(got) != 3 || got[0] != argv[0] {
		t.Fatalf("a turned-off sandbox must not wrap: %v", got)
	}
	if Available() {
		return
	}
	if got := Wrap(argv, ".", nil, nil); len(got) != 3 {
		t.Fatalf("an unavailable sandbox must not wrap: %v", got)
	}
}
