package main

import (
	"os"
	"path/filepath"
	"time"
)

type projectEntry struct {
	Root     string `json:"root"`
	LastSeen int64  `json:"lastSeen"`
	Label    string `json:"label,omitempty"`
	Mode     string `json:"mode,omitempty"` // last permission mode chosen for this folder
}

func projectsFile() string { return filepath.Join(InfuseHome(), "projects.json") }

func loadProjects() []projectEntry {
	var list []projectEntry
	_ = readJSON(projectsFile(), &list)
	if list == nil {
		list = []projectEntry{}
	}
	return list
}

func saveProjects(list []projectEntry) error { return writeJSON(projectsFile(), list) }

// RememberProject records projects the filter has seen, for the panel. A known project keeps the
// position the user gave it; a new one goes first.
func RememberProject(root string) {
	list := loadProjects()
	for i := range list {
		if list[i].Root == root {
			list[i].LastSeen = time.Now().UnixMilli()
			_ = saveProjects(list)
			return
		}
	}
	list = append([]projectEntry{{Root: root, LastSeen: time.Now().UnixMilli()}}, list...)
	if len(list) > 50 {
		list = list[:50]
	}
	_ = saveProjects(list)
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return os.Args[0]
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
