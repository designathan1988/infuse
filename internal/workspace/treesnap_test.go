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

// A rewrite that keeps the byte length and restores the modification time is still a change: the
// digest comparison reads the content, never the file's metadata alone.
func TestTurnChangesSeesAStampedRewrite(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(work, "src", "sum.test.js")
	if err := os.WriteFile(path, []byte("test('sums', () => { assert(1 + 1 === 2); });\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if err := SnapshotTurn(root, work, "s1", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test('sums', () => { assert(1 + 1 === 3); });\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	changes := TurnChanges(root, work, "s1", 1)
	if len(changes) != 1 || changes[0].Path != "src/sum.test.js" {
		t.Fatalf("the stamped rewrite was invisible: %+v", changes)
	}
}
