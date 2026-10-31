package store

import (
	"os"
	"path/filepath"
	"testing"
)

type doc struct {
	Versioned
	Name string `json:"name"`
}

func TestUpgradeStampsAndMigrates(t *testing.T) {
	var d doc
	ran := false
	changed, err := d.Upgrade(map[int]func() error{0: func() error { ran = true; d.Name = "migrated"; return nil }})
	if err != nil || !changed || !ran || d.SchemaVersion != Version || d.Name != "migrated" {
		t.Fatalf("changed=%v ran=%v doc=%+v err=%v", changed, ran, d, err)
	}
	changed, err = d.Upgrade(nil)
	if err != nil || changed {
		t.Fatalf("current document rewritten: changed=%v err=%v", changed, err)
	}
}

func TestNewerDocumentIsLeftAlone(t *testing.T) {
	d := doc{Versioned: Versioned{SchemaVersion: Version + 1}}
	changed, err := d.Upgrade(map[int]func() error{0: func() error { t.Fatal("migration ran"); return nil }})
	if err != nil || changed || d.SchemaVersion != Version+1 {
		t.Fatalf("newer document changed: %v %v %+v", changed, err, d)
	}
}

func TestLoadListMigratesBareArray(t *testing.T) {
	file := filepath.Join(t.TempDir(), "list.json")
	if err := os.WriteFile(file, []byte(`[{"name":"a"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	list, err := LoadList[doc](file)
	if err != nil || len(list.Items) != 1 || list.Items[0].Name != "a" || list.SchemaVersion != 0 {
		t.Fatalf("%+v %v", list, err)
	}
	if err := SaveList(file, list); err != nil {
		t.Fatal(err)
	}
	back, err := LoadList[doc](file)
	if err != nil || back.SchemaVersion != Version || len(back.Items) != 1 {
		t.Fatalf("%+v %v", back, err)
	}
}
