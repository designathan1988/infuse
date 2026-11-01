package config

// The project registry: the folders infuse has seen, in the order the user arranged them.

import (
	"infuse/internal/core"
	"infuse/internal/store"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type ProjectEntry struct {
	Root     string `json:"root"`
	LastSeen int64  `json:"lastSeen"`
	Label    string `json:"label,omitempty"`
	Mode     string `json:"mode,omitempty"` // last permission mode chosen for this folder
}

func ProjectsFile() string { return filepath.Join(core.InfuseHome(), "projects.json") }

// RootKey is the identity of a project folder: slash style, duplicate separators and (on Windows)
// letter case must not create a second entry for the same folder.
func RootKey(root string) string {
	p := filepath.Clean(root)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

func dedupeProjects(list []ProjectEntry) []ProjectEntry {
	out := make([]ProjectEntry, 0, len(list))
	seen := map[string]bool{}
	for _, p := range list {
		key := RootKey(p.Root)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, p)
	}
	return out
}

func LoadProjects() []ProjectEntry {
	list, err := store.LoadList[ProjectEntry](ProjectsFile())
	if err != nil || list.Items == nil {
		return []ProjectEntry{}
	}
	items := dedupeProjects(list.Items)
	if len(items) != len(list.Items) {
		_ = SaveProjects(items)
	}
	return items
}

func SaveProjects(list []ProjectEntry) error {
	return store.SaveList(ProjectsFile(), store.List[ProjectEntry]{Items: dedupeProjects(list)})
}

// RememberProject records projects the filter has seen, for the panel. A known project keeps the
// position the user gave it; a new one goes first.
func RememberProject(root string) {
	key := RootKey(root)
	list := LoadProjects()
	for i := range list {
		if RootKey(list[i].Root) == key {
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
