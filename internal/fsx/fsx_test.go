package fsx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectWalksSourceOnly(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src", "node_modules", ".git", ".infuse", ".github", "build"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	var dirs []string
	if err := Walk(root, Project, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != root {
			dirs = append(dirs, filepath.Base(p))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || dirs[0] != "src" {
		t.Fatalf("project walk saw %v", dirs)
	}
}

func TestSnapshotKeepsHiddenProjectDirs(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"src", ".github", "node_modules", "build"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	seen := map[string]bool{}
	if err := Walk(root, Snapshot, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != root {
			seen[filepath.Base(p)] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !seen[".github"] || !seen["src"] || seen["node_modules"] || seen["build"] {
		t.Fatalf("snapshot walk saw %v", seen)
	}
}

func TestIgnoredPathChecksEveryComponent(t *testing.T) {
	root := filepath.Join("C:", "proj")
	if IgnoredPath(root, filepath.Join(root, "src", "a.go")) {
		t.Fatal("source path ignored")
	}
	if !IgnoredPath(root, filepath.Join(root, "node_modules", "x", "a.js")) {
		t.Fatal("dependency path allowed")
	}
	if !IgnoredPath(root, filepath.Join(root, ".github", "a.yml")) {
		t.Fatal("hidden path allowed")
	}
}
