package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// The blob store keeps the newest turns of a session and drops content no kept manifest refers to.
func TestPruneTreeKeepsRecentTurnsAndReferencedBlobs(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(work, "a.txt")
	var firstHash string
	for turn := 1; turn <= keptManifests+5; turn++ {
		content := []byte{byte('a' + turn%26), byte('0' + turn%10)}
		if err := os.WriteFile(file, content, 0o644); err != nil {
			t.Fatal(err)
		}
		if turn == 1 {
			firstHash = hashBytes(content)
		}
		if err := SnapshotTurn(root, work, "s1", turn); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(treeDir(root), "s1"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != keptManifests {
		t.Fatalf("kept %d manifests, want %d", len(entries), keptManifests)
	}
	if _, err := os.Stat(blobPath(root, firstHash)); !os.IsNotExist(err) {
		t.Fatal("the blob of a dropped turn survived")
	}
	// An orphan blob (no manifest refers to it) is removed too.
	orphan := blobPath(root, hashBytes([]byte("orphan")))
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("orphan"), 0o644); err != nil {
		t.Fatal(err)
	}
	PruneTree(root)
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("an unreferenced blob survived the prune")
	}
}
