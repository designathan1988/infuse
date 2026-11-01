package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestResolveDirNeverEmpty is the regression for the terminal that failed on the home screen: with no
// project root and no requested folder the shell must still get a real, existing working directory.
func TestResolveDirNeverEmpty(t *testing.T) {
	dir, err := ResolveDir("", "")
	if err != nil || dir == "" {
		t.Fatalf("empty root and cwd must resolve to a directory, got %q, %v", dir, err)
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		t.Fatalf("resolved directory %q is not a directory", dir)
	}
	valid := t.TempDir()
	if got, err := ResolveDir("", valid); err != nil || got != valid {
		t.Fatalf("a valid requested directory must be kept, got %q, %v", got, err)
	}
	if got, err := ResolveDir(valid, filepath.Join(valid, "does-not-exist")); err != nil || got != valid {
		t.Fatalf("an invalid request must fall back to the project, got %q, %v", got, err)
	}
}

// TestOpenWithNoDirectory is the same defect at the shell boundary: Open never receives an empty cwd.
func TestOpenWithNoDirectory(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("no console in CI")
	}
	term, err := Open("", "", 80, 24)
	if err != nil {
		t.Skip("pty unavailable: " + err.Error())
	}
	defer Close(term.ID)
	if term.Cwd == "" {
		t.Fatal("the shell started without a working directory")
	}
	_, _ = term.Write([]byte("echo no-dir-$((40+2))\r"))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(term.Output(), "no-dir-42") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no echo from the shell; got %q", term.Output())
}

func TestTerminalEcho(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("no console in CI")
	}
	dir := t.TempDir()
	term, err := Open(dir, dir, 80, 24)
	if err != nil {
		t.Skip("pty unavailable: " + err.Error())
	}
	defer Close(term.ID)
	_, _ = term.Write([]byte("echo infuse-term-$((40+2))\r"))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(term.Output(), "infuse-term-42") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no echo from the shell; got %q", term.Output())
}
