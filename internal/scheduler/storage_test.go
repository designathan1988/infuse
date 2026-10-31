package scheduler

import (
	"infuse/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskCarriesSchemaVersion(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	task, err := Save(Task{Name: "daily-check", Prompt: "run the tests", Root: root, Preset: "daily", Active: true})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(Dir(), task.Name+".json")
	legacy := []byte(`{"name":"daily-check","prompt":"run the tests","root":"` + filepath.ToSlash(root) + `","preset":"daily","active":true}`)
	if err := os.WriteFile(file, legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(task.Name)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != store.Version {
		t.Fatalf("schema version %d", loaded.SchemaVersion)
	}
}
