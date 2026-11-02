package projectmap

// The base's declared structure is the project's intended architecture. Its folders appear in the map
// beside the ones the code actually created, marked as intended, so the gap between the intended and
// the actual architecture is visible before the first file exists. The base package owns the base;
// this file is the copy the map reads, so the map does not depend on it.

import (
	"encoding/json"
	"infuse/internal/core"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// FolderRole is one declared folder of a base and what lives in it.
type FolderRole struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

// Layer is one declared layer and the layers it may import.
type Layer struct {
	Path      string   `json:"path"`
	MayImport []string `json:"mayImport"`
}

// Contract is what the map needs of the chosen base.
type Contract struct {
	Layout       []FolderRole `json:"layout,omitempty"`
	Folders      []string     `json:"folders,omitempty"`
	Layers       []Layer      `json:"layers,omitempty"`
	Public       []string     `json:"public,omitempty"` // folders whose exports are the product's surface
	MaxFileLines int          `json:"maxFileLines,omitempty"`
}

// IntendedFile records the chosen base's declared structure, written by the scaffold.
func IntendedFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "map", "intended.json")
}

// WriteContract records the base's declared structure.
func WriteContract(root string, c Contract) error {
	if len(c.Layout) == 0 && len(c.Folders) == 0 {
		_ = os.Remove(IntendedFile(root))
		return nil
	}
	return core.WriteJSON(IntendedFile(root), c)
}

// LoadContract reads the declared structure (empty when no base was chosen). A file from an older
// build is the plain list of folders.
func LoadContract(root string) Contract {
	b, err := os.ReadFile(IntendedFile(root))
	if err != nil {
		return Contract{}
	}
	var c Contract
	if json.Unmarshal(b, &c) == nil {
		return c
	}
	var old []FolderRole
	if json.Unmarshal(b, &old) == nil {
		c.Layout = old
	}
	return c
}

// ApplyIntended merges the base's declared folders into a freshly built map.
func ApplyIntended(root string, m *ProjectMap) { applyContract(m, LoadContract(root)) }

// applyContract gives each declared folder its role. One with no code yet — declared only, or created
// empty by the scaffold — shows as intended; the mark leaves as soon as the folder holds code.
func applyContract(m *ProjectMap, c Contract) {
	if m == nil || m.Dirs == nil {
		return
	}
	for _, f := range c.Layout {
		d, ok := m.Dirs[f.Path]
		if !ok {
			m.Dirs[f.Path] = &DirInfo{Path: f.Path, Files: []string{}, Subdirs: []string{}, Intended: true, Role: f.Role}
			linkSubdir(m, f.Path)
			continue
		}
		d.Role = f.Role
		d.Intended = len(d.Files) == 0 && len(d.Subdirs) == 0
	}
}

// linkSubdir lists a folder in its parent, creating the chain up to the root.
func linkSubdir(m *ProjectMap, rel string) {
	if rel == "." {
		return
	}
	parent := path.Dir(rel)
	p, ok := m.Dirs[parent]
	if !ok {
		p = &DirInfo{Path: parent, Files: []string{}, Subdirs: []string{}}
		m.Dirs[parent] = p
		linkSubdir(m, parent)
	}
	name := path.Base(rel)
	if !core.Contains(p.Subdirs, name) {
		p.Subdirs = append(p.Subdirs, name)
		sort.Strings(p.Subdirs)
	}
}

// Drift is one place where the code departs from the base.
type Drift struct {
	Kind   string `json:"kind"` // undeclared-folder | layer-violation | intended-only
	Path   string `json:"path"`
	Detail string `json:"detail"`
}

// Within reports whether rel lies in one of the folders.
func Within(rel string, folders []string) bool {
	return owningFolder(rel, folders) != ""
}

// owningFolder is the longest of folders that contains rel ("" when none does).
func owningFolder(rel string, folders []string) string {
	best := ""
	for _, f := range folders {
		f = strings.Trim(strings.TrimPrefix(filepath.ToSlash(f), "./"), "/")
		if (rel == f || strings.HasPrefix(rel, f+"/")) && len(f) > len(best) {
			best = f
		}
	}
	return best
}

// LayerViolation says why rel, in its layer, may not import target ("" when it may or no layer
// governs them).
func (c Contract) LayerViolation(rel, target string) string {
	paths := make([]string, 0, len(c.Layers))
	for _, l := range c.Layers {
		paths = append(paths, l.Path)
	}
	from, to := owningFolder(rel, paths), owningFolder(target, paths)
	if from == "" || to == "" || from == to {
		return ""
	}
	for _, l := range c.Layers {
		if l.Path == from && !core.Contains(l.MayImport, to) {
			return from + " may not import " + to
		}
	}
	return ""
}

// ImportViolation checks what a text imports against the layers, before the file exists: the rule
// broken and the import that broke it, or empty strings.
func (c Contract) ImportViolation(root, rel, text string) (string, string) {
	l, ok := langOf(rel)
	if !ok {
		return "", ""
	}
	module := goModule(root)
	for _, spec := range extractImports(text, l) {
		if why := c.LayerViolation(rel, importTarget(module, rel, spec, l)); why != "" {
			return why, spec
		}
	}
	return "", ""
}

// importTarget is the project path an import points at ("" when it leaves the project or the language
// does not name paths).
func importTarget(module, rel, spec, l string) string {
	switch l {
	case "go":
		if module != "" && (spec == module || strings.HasPrefix(spec, module+"/")) {
			return strings.TrimPrefix(strings.TrimPrefix(spec, module), "/")
		}
	case "ts", "js", "vue", "svelte":
		if strings.HasPrefix(spec, ".") {
			return path.Join(path.Dir(rel), spec)
		}
	case "c", "cpp":
		return path.Join(path.Dir(rel), spec)
	case "py":
		if !strings.HasPrefix(spec, ".") {
			return strings.ReplaceAll(spec, ".", "/")
		}
		dots := len(spec) - len(strings.TrimLeft(spec, "."))
		return path.Join(path.Dir(rel), strings.Repeat("../", dots-1), strings.ReplaceAll(spec[dots:], ".", "/"))
	}
	return ""
}

// computeDrift compares the code with the contract: code outside the declared folders, imports across
// layers the base forbids, folders the base declares that hold nothing yet.
func computeDrift(m *ProjectMap, c Contract) []Drift {
	if len(c.Folders) == 0 && len(c.Layers) == 0 && len(c.Layout) == 0 {
		return nil
	}
	var out []Drift
	seen := map[string]bool{}
	for _, f := range sortedFiles(m) {
		if nonCodeLangs[f.Lang] {
			continue
		}
		dir := path.Dir(f.Path)
		if dir != "." && len(c.Folders) > 0 && !Within(f.Path, c.Folders) && !seen[dir] {
			seen[dir] = true
			out = append(out, Drift{"undeclared-folder", dir, "code in a folder the base does not declare"})
		}
		for _, imp := range f.Imports {
			if m.Files[imp] == nil {
				continue
			}
			if why := c.LayerViolation(f.Path, imp); why != "" {
				out = append(out, Drift{"layer-violation", f.Path, why + " (imports " + imp + ")"})
			}
		}
	}
	for _, l := range c.Layout {
		if d := m.Dirs[l.Path]; d != nil && d.Intended {
			out = append(out, Drift{"intended-only", l.Path, "declared by the base, no code yet"})
		}
	}
	return out
}

func sortedFiles(m *ProjectMap) []*FileInfo {
	names := make([]string, 0, len(m.Files))
	for p := range m.Files {
		names = append(names, p)
	}
	sort.Strings(names)
	out := make([]*FileInfo, 0, len(names))
	for _, p := range names {
		out = append(out, m.Files[p])
	}
	return out
}
