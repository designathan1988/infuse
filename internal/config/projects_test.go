package config

import (
	"path/filepath"
	"testing"
)

func TestProjectsDedupeEquivalentFolders(t *testing.T) {
	testHome(t)
	root := filepath.Join("home", "project")
	if err := SaveProjects([]ProjectEntry{{Root: root}, {Root: root + string(filepath.Separator)}}); err != nil {
		t.Fatal(err)
	}
	got := LoadProjects()
	if len(got) != 1 {
		t.Fatalf("LoadProjects = %+v, want one entry", got)
	}
	RememberProject(root)
	if got = LoadProjects(); len(got) != 1 {
		t.Fatalf("RememberProject duplicated: %+v", got)
	}
}
