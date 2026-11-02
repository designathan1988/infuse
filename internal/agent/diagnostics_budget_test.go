package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteDoesNotWaitForTheChecker(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, done := diagnosticsWithin(root, p, 5*time.Second); !done {
		t.Fatal("a file with no checker answers at once")
	}
	diagnosticsBusy.Store(root, true) // a checker of an earlier write is still running
	defer diagnosticsBusy.Delete(root)
	start := time.Now()
	if _, done := diagnosticsWithin(root, p, 5*time.Second); done || time.Since(start) > time.Second {
		t.Fatalf("a write waited for a running checker: %v %v", done, time.Since(start))
	}
}
