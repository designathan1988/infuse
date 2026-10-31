package knowledge

import (
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"testing"
)

func TestMemoryFileMovesToEnvelope(t *testing.T) {
	file := t.TempDir() + "/memory.json"
	legacy := []MemoryItem{{ID: "m1", Type: "fato", Text: "x"}}
	if err := core.WriteJSON(file, legacy); err != nil {
		t.Fatal(err)
	}
	items := LoadMemory(file)
	if len(items) != 1 || items[0].ID != "m1" {
		t.Fatalf("%+v", items)
	}
	if err := SaveMemory(file, items); err != nil {
		t.Fatal(err)
	}
	list, err := store.LoadList[MemoryItem](file)
	if err != nil || list.SchemaVersion != store.Version || len(list.Items) != 1 {
		t.Fatalf("%+v %v", list, err)
	}
	b, _ := os.ReadFile(file)
	if len(b) == 0 {
		t.Fatal("empty file")
	}
}
