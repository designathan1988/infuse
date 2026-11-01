package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// Revert restores the tree recorded at the start of the turn, so a file deleted by a shell command
// comes back and a file created after the snapshot is removed.
func TestRestoreTurnBringsBackDeletions(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "src", "a.js"), []byte("export const a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SnapshotTurn(root, work, "s1", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(work, "src", "a.js")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "src", "later.js"), []byte("export const b = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RestoreTurn(root, work, "s1", 1)
	if b, err := os.ReadFile(filepath.Join(work, "src", "a.js")); err != nil || string(b) != "export const a = 1;\n" {
		t.Fatalf("deleted file not restored: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(work, "src", "later.js")); !os.IsNotExist(err) {
		t.Fatal("a file created after the snapshot was not removed")
	}
}
