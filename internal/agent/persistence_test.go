package agent

import (
	"encoding/json"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionFileCarriesSchemaVersion(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.save()
	file := filepath.Join(sessionsDir(root), s.ID+".json")
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["schemaVersion"].(float64) != float64(store.Version) {
		t.Fatalf("not stamped: %v", raw["schemaVersion"])
	}
}
