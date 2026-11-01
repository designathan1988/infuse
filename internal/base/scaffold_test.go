package base

import (
	"os"
	"path/filepath"
	"testing"
)

// A starter base scaffolds its folders, records the choice and documents the structure.
func TestScaffoldCreatesTheBaseStructure(t *testing.T) {
	root := t.TempDir()
	b, err := Scaffold(root, "web")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range b.Folders {
		if st, err := os.Stat(filepath.Join(root, filepath.FromSlash(d))); err != nil || !st.IsDir() {
			t.Fatalf("folder %s missing: %v", d, err)
		}
	}
	loaded := Load(root)
	if loaded == nil || loaded.Name != "web" {
		t.Fatalf("%+v", loaded)
	}
	if _, err := os.Stat(filepath.Join(root, "docs", "architecture", "base.md")); err != nil {
		t.Fatal("the base document was not written")
	}
	if _, err := Scaffold(root, "nope"); err == nil {
		t.Fatal("an unknown base was scaffolded")
	}
}
