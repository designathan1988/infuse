package config

// The project registry: the folders infuse has seen, in the order the user arranged them.

import (
	"infuse/internal/core"
	"path/filepath"
	"time"
)

type ProjectEntry struct {
	Root     string `json:"root"`
	LastSeen int64  `json:"lastSeen"`
	Label    string `json:"label,omitempty"`
	Mode     string `json:"mode,omitempty"` // last permission mode chosen for this folder
}

func ProjectsFile() string { return filepath.Join(core.InfuseHome(), "projects.json") }

func LoadProjects() []ProjectEntry {
	var list []ProjectEntry
	_ = core.ReadJSON(ProjectsFile(), &list)
	if list == nil {
		list = []ProjectEntry{}
	}
	return list
}

func SaveProjects(list []ProjectEntry) error { return core.WriteJSON(ProjectsFile(), list) }

// RememberProject records projects the filter has seen, for the panel. A known project keeps the
// position the user gave it; a new one goes first.
func RememberProject(root string) {
	list := LoadProjects()
	for i := range list {
		if list[i].Root == root {
			list[i].LastSeen = time.Now().UnixMilli()
			_ = SaveProjects(list)
			return
		}
	}
	list = append([]ProjectEntry{{Root: root, LastSeen: time.Now().UnixMilli()}}, list...)
	if len(list) > 50 {
		list = list[:50]
	}
	_ = SaveProjects(list)
}
