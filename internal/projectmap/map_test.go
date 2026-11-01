package projectmap

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMapFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Go imports resolve inside the module, so dependents are no longer empty for a Go project.
func TestGoDependentsResolve(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "go.mod", "module example.com/app\n\ngo 1.26\n")
	writeMapFile(t, root, "internal/store/store.go", "package store\n\nfunc New() int { return 1 }\n")
	writeMapFile(t, root, "cmd/app/main.go", "package main\n\nimport \"example.com/app/internal/store\"\n\nfunc main() { _ = store.New() }\n")
	m := BuildMap(root)
	if main := m.Files["cmd/app/main.go"]; main == nil || len(main.Imports) == 0 || main.Imports[0] != "internal/store/store.go" {
		t.Fatalf("module import not resolved: %+v", main)
	}
	store := m.Files["internal/store/store.go"]
	if store == nil || len(store.Dependents) == 0 || store.Dependents[0] != "cmd/app/main.go" {
		t.Fatalf("dependent not recorded: %+v", store)
	}
}

// C# using statements resolve through the namespace each file declares.
func TestCSharpDependentsResolve(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/Data/Repo.cs", "namespace App.Data;\npublic class Repo {}\n")
	writeMapFile(t, root, "src/App/Service.cs", "using App.Data;\nnamespace App.Services { public class S { private Repo r; } }\n")
	m := BuildMap(root)
	repo := m.Files["src/Data/Repo.cs"]
	if repo == nil || len(repo.Dependents) == 0 || repo.Dependents[0] != "src/App/Service.cs" {
		t.Fatalf("C# import not resolved: %+v", repo)
	}
}

// Rust use/mod paths resolve to the file they name.
func TestRustDependentsResolve(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/lib.rs", "mod greet;\n\npub fn run() {}\n")
	writeMapFile(t, root, "src/greet.rs", "pub fn hi() {}\n")
	writeMapFile(t, root, "src/use_greet.rs", "use crate::greet::hi;\n\npub fn go() { hi(); }\n")
	m := BuildMap(root)
	greet := m.Files["src/greet.rs"]
	if greet == nil || len(greet.Dependents) < 2 {
		t.Fatalf("rust imports not resolved: %+v", greet)
	}
	found := false
	for _, d := range greet.Dependents {
		found = found || d == "src/use_greet.rs"
	}
	if !found {
		t.Fatalf("dependent missing: %v", greet.Dependents)
	}
}
