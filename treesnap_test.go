package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The case that showed "−0" for everything: files created, then changed by shell commands and by
// edits that remove lines. Every removal must be counted, whatever made it.
func TestChangesCountRemovalsWhateverMadeThem(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	w := func(p, s string) {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		_ = os.WriteFile(filepath.Join(root, p), []byte(s), 0o644)
	}
	w("README.md", "# projeto\nlinha a\nlinha b\n")
	w("velho.js", "um\ndois\ntres\n")
	w("node_modules/dep/index.js", "ignorado\n")
	snapshotTurn(root, root, "s1", 1) // the turn starts

	w("cli.js", "a\nb\nc\nd\n")                    // the agent creates
	w("test/auxiliares.js", "x\ny\n")              // the agent creates
	_ = os.Remove(filepath.Join(root, "velho.js")) // rm by the shell
	_ = os.Rename(filepath.Join(root, "test/auxiliares.js"), filepath.Join(root, "test/servidor-simulado.js"))
	w("README.md", "# projeto\nlinha b\n") // sed -i removing a line

	files := changesByPath(LocalChanges(root, root, "s1", "all"))
	if f := files["velho.js"]; f.Status != "D" || f.Removed != 3 {
		t.Fatalf("file removed by the shell: %+v", f)
	}
	if f := files["README.md"]; f.Status != "M" || f.Removed != 1 || f.Added != 0 {
		t.Fatalf("line removed by sed: %+v", f)
	}
	if f := files["test/servidor-simulado.js"]; f.Status != "A" || f.Added != 2 {
		t.Fatalf("renamed file missing: %+v", f)
	}
	if _, ok := files["test/auxiliares.js"]; ok {
		t.Fatal("a file created and renamed in the same scope is not a change")
	}
	if _, ok := files["node_modules/dep/index.js"]; ok {
		t.Fatal("dependencies counted")
	}

	// next turn: edits that remove lines of a file created before
	snapshotTurn(root, root, "s1", 2)
	w("cli.js", "a\nD\n")
	turn := changesByPath(LocalChanges(root, root, "s1", "turn"))
	if f := turn["cli.js"]; f.Status != "M" || f.Removed != 3 || f.Added != 1 || len(turn) != 1 {
		t.Fatalf("this turn's edit: %+v (all: %+v)", f, turn)
	}
	if d := LocalDiff(root, root, "s1", "turn", "cli.js"); !strings.Contains(d, "-b") || !strings.Contains(d, "-c") || !strings.Contains(d, "+D") {
		t.Fatal(d)
	}
	// since the start, cli.js is still a new file
	if f := changesByPath(LocalChanges(root, root, "s1", "all"))["cli.js"]; f.Status != "A" || f.Added != 2 {
		t.Fatalf("%+v", f)
	}

	// discarding brings back what each scope started from
	if err := LocalDiscard(root, root, "s1", "turn", "cli.js"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "cli.js")); string(b) != "a\nb\nc\nd\n" {
		t.Fatalf("%q", b)
	}
	if err := LocalDiscard(root, root, "s1", "all", "velho.js"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "velho.js")); string(b) != "um\ndois\ntres\n" {
		t.Fatalf("deleted file not restored: %q", b)
	}
}

// A project with sessions from before snapshots: files written after its first session count as new.
func TestChangesWithoutSnapshotSeeShellFiles(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Turn = 1
	s.checkpointBeforeWrite(filepath.Join(root, "a.js"))
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("1\n"), 0o644)
	s.save()
	_ = os.WriteFile(filepath.Join(root, "gerado.js"), []byte("1\n2\n"), 0o644) // made by a shell command
	files := changesByPath(LocalChanges(root, root, s.ID, "all"))
	if files["gerado.js"].Status != "A" || files["gerado.js"].Added != 2 || files["a.js"].Status != "A" {
		t.Fatalf("%+v", files)
	}
}

func changesByPath(ch map[string]any) map[string]fileChange {
	out := map[string]fileChange{}
	for _, f := range ch["files"].([]fileChange) {
		out[f.Path] = f
	}
	return out
}

// After rm in the same command line, the folder summary appended to ls describes the folder as it is now.
func TestListingAfterRemovalIsFresh(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "legado.js"), []byte("export const a = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "src", "soma.js"), []byte("export const b = 2;\n"), 0o644)
	WriteMap(root, BuildMap(root)) // the map as it was before the command
	_ = os.Remove(filepath.Join(root, "src", "legado.js"))
	out := buildTrailer(root, root, "rm src/legado.js && ls src", nil, 0, &RuleSet{}, nil)
	if strings.Contains(out, "legado") || !strings.Contains(out, "src: 1 files") {
		t.Fatalf("stale summary: %s", out)
	}
	if mayChangeTree("ls -la src && cat a.txt") || !mayChangeTree("mv a b && ls") {
		t.Fatal("mayChangeTree")
	}
}
