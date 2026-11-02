package base

import (
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"strings"
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

// Choosing a base pre-fills the map: the base's folders appear in it as intended directories, with the
// role the base gives each one, before any code exists. (The scaffold itself now writes starter code,
// so this holds for the choice; TestScaffoldPrefillsTheMapFromTheStarter covers the scaffold.)
func TestChosenBasePrefillsTheMapWithTheIntendedStructure(t *testing.T) {
	root := t.TempDir()
	if err := Save(root, "api", nil); err != nil {
		t.Fatal(err)
	}
	b := *Load(root)
	if len(b.Layout) == 0 {
		t.Fatal("the starter base declares no layout")
	}
	m := projectmap.LoadOrBuildMap(root) // the consumer's view: generated folders merged with the intended ones
	found := 0
	for _, f := range b.Layout {
		d, ok := m.Dirs[f.Path]
		if !ok {
			continue
		}
		if !d.Intended || d.Role != f.Role {
			t.Fatalf("folder %s: intended=%v role=%q", f.Path, d.Intended, d.Role)
		}
		found++
	}
	if found != len(b.Layout) {
		t.Fatalf("%d of %d intended folders in the map", found, len(b.Layout))
	}
	if line := projectmap.DescribeDir(m, "internal/store"); !strings.Contains(line, "intended") {
		t.Fatalf("describe does not mark the intended folder: %s", line)
	}
	// a folder that gains code stops being marked intended
	_ = os.MkdirAll(filepath.Join(root, "internal", "store"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "internal", "store", "store.go"), []byte("package store\n"), 0o644)
	// the watcher (or the map refresh) rebuilds it after the write, and the mark is gone
	projectmap.WriteMap(root, projectmap.BuildMap(root))
	m2 := projectmap.LoadOrBuildMap(root)
	if d := m2.Dirs["internal/store"]; d == nil || d.Intended {
		t.Fatalf("a folder with code is still intended: %+v", d)
	}
	// and the map document lists the intended folders beside the real ones
	projectmap.WriteMap(root, m2)
	md, err := os.ReadFile(filepath.Join(root, ".infuse", "map", "MAP.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "intended") {
		t.Fatal("MAP.md does not mention the intended folders")
	}
}
