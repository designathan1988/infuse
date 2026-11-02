package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// An edit that keeps the size and restores the modification time is still a change: the audit hashes
// the content, not the file's metadata.
func TestSameSizeEditWithRestoredTimeIsSeen(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.js")
	if err := os.WriteFile(p, []byte("const a = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if err := SnapshotTurn(root, root, "s", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("const a = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, old, old); err != nil {
		t.Fatal(err)
	}
	if got := TurnChanges(root, root, "s", 1); len(got) != 1 || got[0].Path != "a.js" {
		t.Fatalf("the edit is invisible to the audit: %+v", got)
	}
}

func protectedFile(t *testing.T, root, rel string, content []byte) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Deleting the criteria is a change to the protected tree, not the absence of one.
func TestDeletedProtectedFileIsReported(t *testing.T) {
	root := t.TempDir()
	p := protectedFile(t, root, ".infuse/oracle/checks.json", []byte(`{"checks":[]}`))
	if err := SnapshotTurn(root, root, "s", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if got := TurnProtectedChanges(root, "s", 1); len(got) != 1 || got[0] != ".infuse/oracle/checks.json" {
		t.Fatalf("a deletion must be reported: %v", got)
	}
}

// A protected file over any size stays hashed.
func TestLargeProtectedFileIsHashed(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, 2<<20)
	p := protectedFile(t, root, ".infuse/evidence/run/out.log", big)
	if err := SnapshotTurn(root, root, "s", 1); err != nil {
		t.Fatal(err)
	}
	big[len(big)-1] = 'x'
	if err := os.WriteFile(p, big, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := TurnProtectedChanges(root, "s", 1); len(got) != 1 {
		t.Fatalf("a large protected file was not compared: %v", got)
	}
}

// The executor's refresh accepts only what it names: a criteria file changed by the agent survives the
// refresh as a reported change.
func TestRefreshAcceptsOnlyWhatInfuseWrote(t *testing.T) {
	root := t.TempDir()
	spec := protectedFile(t, root, ".infuse/oracle/checks.json", []byte(`{"checks":[]}`))
	if err := SnapshotTurn(root, root, "s", 1); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spec, []byte(`{"checks":[{"id":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	protectedFile(t, root, ".infuse/evidence/run/verdict.json", []byte("{}"))
	RefreshTurnProtected(root, "s", 1, func(rel string) bool { return rel == ".infuse/evidence/run/verdict.json" })
	if got := TurnProtectedChanges(root, "s", 1); len(got) != 1 || got[0] != ".infuse/oracle/checks.json" {
		t.Fatalf("got %v", got)
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
