package config

import (
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func TestSettingsGainSchemaVersion(t *testing.T) {
	testHome(t)
	if err := core.WriteJSON(SettingsFile(), map[string]any{"model": "claude-sonnet-5-5", "language": "en"}); err != nil {
		t.Fatal(err)
	}
	s := LoadSettings()
	if s.SchemaVersion != store.Version {
		t.Fatalf("schema version %d", s.SchemaVersion)
	}
	var raw map[string]any
	b, _ := os.ReadFile(SettingsFile())
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schemaVersion"].(float64) != float64(store.Version) {
		t.Fatalf("file not stamped: %v", raw["schemaVersion"])
	}
}

func TestProjectsFileMovesToEnvelope(t *testing.T) {
	testHome(t)
	legacy := []ProjectEntry{{Root: "C:/a"}, {Root: "C:/b"}}
	if err := core.WriteJSON(ProjectsFile(), legacy); err != nil {
		t.Fatal(err)
	}
	got := LoadProjects()
	// one spelling per folder: mixed separators are canonicalized, never duplicated
	if len(got) != 2 || filepath.ToSlash(got[0].Root) != "C:/a" {
		t.Fatalf("%+v", got)
	}
	if err := SaveProjects(got); err != nil {
		t.Fatal(err)
	}
	list, err := store.LoadList[ProjectEntry](ProjectsFile())
	if err != nil || list.SchemaVersion != store.Version || len(list.Items) != 2 {
		t.Fatalf("%+v %v", list, err)
	}
}
