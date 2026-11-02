package projectmap

// The map on disk: .infuse/map/map.json, MAP.md and one document per folder. They are versioned, so
// they carry nothing that changes without the code changing, and a file is rewritten only when its
// content differs.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

func MapFile(root string) string { return filepath.Join(root, core.InfuseDirName, "map", "map.json") }

func mapDocFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "map", "MAP.md")
}

func dirsRoot(root string) string { return filepath.Join(root, core.InfuseDirName, "map", "dirs") }

// DirFile is the document of one folder (SPEC §6: the map, per directory).
func DirFile(root, rel string) string {
	if rel == "" || rel == "." {
		return filepath.Join(dirsRoot(root), ".root.md")
	}
	return filepath.Join(dirsRoot(root), filepath.FromSlash(rel)+".md")
}

// WriteMap stores a whole map. MAP.md and the folder documents are regenerated with it, so none of
// them can fall behind map.json.
func WriteMap(root string, m *ProjectMap) {
	ApplyIntended(root, m)
	if m.partial {
		return
	}
	writeMapJSON(root, m)
	writeIfChanged(mapDocFile(root), mapDocument(m))
	writeDirDocs(root, m, sortedDirs(m))
	pruneDirDocs(root, m)
	remember(root, m)
}

var reGeneratedAt = regexp.MustCompile(`"generatedAt":\s*\d+`)

// writeMapJSON skips the write when only the generation time would change: a versioned map moves
// when the code moves.
func writeMapJSON(root string, m *ProjectMap) {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return
	}
	if old, err := os.ReadFile(MapFile(root)); err == nil {
		b = append(b, '\n')
		if bytes.Equal(reGeneratedAt.ReplaceAll(old, nil), reGeneratedAt.ReplaceAll(b, nil)) {
			return
		}
	}
	_ = core.WriteJSON(MapFile(root), m)
}

func sortedDirs(m *ProjectMap) []string {
	dirs := make([]string, 0, len(m.Dirs))
	for k := range m.Dirs {
		dirs = append(dirs, k)
	}
	sort.Strings(dirs)
	return dirs
}

// mapDocument is MAP.md: every folder, then the levels of the whole project.
func mapDocument(m *ProjectMap) string {
	var md strings.Builder
	md.WriteString("# Project map\n\nGenerated from the code by infuse; never edit by hand.\n\n")
	for _, d := range sortedDirs(m) {
		md.WriteString(describe(m, d, false) + "\n")
	}
	md.WriteString(LevelsBrief(m, 0))
	appendFeaturesMD(&md, m)
	if len(m.Hotspots) > 0 {
		md.WriteString("\n## Hotspots\n")
		for _, h := range m.Hotspots {
			fmt.Fprintf(&md, "- %s: %s\n", h.Path, h.Reason)
		}
	}
	return md.String()
}

// writeDirDocs refreshes the documents of some folders, and their context files when the project
// turned those on.
func writeDirDocs(root string, m *ProjectMap, dirs []string) {
	context := ContextFilesEnabled(root)
	for _, d := range dirs {
		if _, ok := m.Dirs[d]; !ok {
			_ = os.Remove(DirFile(root, d))
			continue
		}
		doc := describe(m, d, false) + "\n"
		changed := writeIfChanged(DirFile(root, d), doc)
		if context && (changed || !hasContextFiles(root, d)) && isContextDir(m, d) {
			writeContextFiles(root, d, doc)
		}
	}
}

// pruneDirDocs removes the documents of folders that no longer exist.
func pruneDirDocs(root string, m *ProjectMap) {
	base := dirsRoot(root)
	want := map[string]bool{}
	for d := range m.Dirs {
		want[DirFile(root, d)] = true
	}
	_ = filepath.WalkDir(base, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && !want[p] {
			_ = os.Remove(p)
		}
		return nil
	})
}

// writeIfChanged writes text unless the file already holds it, and reports whether it wrote.
func writeIfChanged(file, text string) bool {
	if old, err := os.ReadFile(file); err == nil && string(old) == text {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return false
	}
	return os.WriteFile(file, []byte(text), 0o644) == nil
}

func LoadMap(root string) *ProjectMap {
	var m ProjectMap
	if core.ReadJSON(MapFile(root), &m) != nil {
		return nil
	}
	m.Root = root
	return &m
}

// mapCache keeps the parsed map per project while the map file is unchanged: shell commands read it
// on every call, and re-parsing the JSON each time is wasted work in the gate path.
var mapCache sync.Map // root → mapCacheEntry

type mapCacheEntry struct {
	mod  int64
	size int64
	m    *ProjectMap
}

func remember(root string, m *ProjectMap) {
	if st, err := os.Stat(MapFile(root)); err == nil {
		mapCache.Store(root, mapCacheEntry{st.ModTime().UnixNano(), st.Size(), m})
	}
}

// Peek returns the stored map without ever building one (nil when there is none): the filter path
// cannot pay for a walk of the project. The map it returns is shared and read-only.
func Peek(root string) *ProjectMap {
	st, err := os.Stat(MapFile(root))
	if err != nil {
		return nil
	}
	if v, ok := mapCache.Load(root); ok {
		e := v.(mapCacheEntry)
		if e.mod == st.ModTime().UnixNano() && e.size == st.Size() {
			return e.m
		}
	}
	m := LoadMap(root)
	if m != nil {
		mapCache.Store(root, mapCacheEntry{st.ModTime().UnixNano(), st.Size(), m})
	}
	return m
}

// LoadOrBuildMap returns the stored map, building it the first time. A map written before the feature
// index has no features or routes keys, and is rebuilt once.
func LoadOrBuildMap(root string) *ProjectMap {
	if m := Peek(root); m != nil && m.Features != nil && m.Routes != nil {
		return m
	}
	m := BuildMap(root)
	WriteMap(root, m)
	return m
}
