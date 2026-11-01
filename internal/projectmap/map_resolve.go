package projectmap

// Import resolution: map a dependency to the file it names.

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"infuse/internal/core"
)

var tsExts = []string{".ts", ".tsx", ".mts", ".js", ".jsx", ".mjs", ".cjs", "/index.ts", "/index.tsx", "/index.js"}

func resolveImports(root string, m *ProjectMap) {
	module := goModule(root)
	for _, f := range m.Files {
		var extra []string
		for i, spec := range f.Imports {
			switch f.Lang {
			case "ts", "js":
				resolveTSImport(root, f, i, spec, m)
			case "py":
				resolvePyImport(root, f, i, spec, m)
			case "go":
				if rel, more := resolveGoImport(module, spec, m); rel != "" {
					f.Imports[i] = rel
					extra = append(extra, more...)
				}
			case "rs":
				if rel, ok := resolveRustImport(f.Path, spec, m); ok {
					f.Imports[i] = rel
				}
			}
		}
		f.Imports = dedupe(f.Imports, extra...)
	}
	for _, f := range m.Files {
		for _, imp := range f.Imports {
			if t, ok := m.Files[imp]; ok {
				t.Dependents = append(t.Dependents, f.Path)
			}
		}
	}
	for _, f := range m.Files {
		sort.Strings(f.Dependents)
	}
}

func dedupe(list []string, more ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list)+len(more))
	for _, s := range append(append([]string{}, list...), more...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func resolveTSImport(root string, f *FileInfo, i int, spec string, m *ProjectMap) {
	if !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
		return
	}
	base := filepath.Join(root, filepath.Dir(filepath.FromSlash(f.Path)), filepath.FromSlash(spec))
	stripped := strings.TrimSuffix(base, ".js")
	cands := []string{base}
	for _, e := range tsExts {
		cands = append(cands, stripped+e, base+e)
	}
	for _, c := range cands {
		rel, _ := filepath.Rel(root, c)
		rel = core.ToPosix(rel)
		if _, ok := m.Files[rel]; ok {
			f.Imports[i] = rel
			return
		}
	}
}

func resolvePyImport(root string, f *FileInfo, i int, spec string, m *ProjectMap) {
	dots := 0
	for dots < len(spec) && spec[dots] == '.' {
		dots++
	}
	relPath := strings.ReplaceAll(spec[dots:], ".", "/")
	var base string
	if dots > 0 {
		base = filepath.Join(root, filepath.Dir(filepath.FromSlash(f.Path)), strings.Repeat("../", dots-1), relPath)
	} else {
		base = filepath.Join(root, relPath)
	}
	for _, c := range []string{base + ".py", filepath.Join(base, "__init__.py")} {
		rel, _ := filepath.Rel(root, c)
		rel = core.ToPosix(rel)
		if _, ok := m.Files[rel]; ok {
			f.Imports[i] = rel
			return
		}
	}
}

// resolveGoImport maps a module import to the package's files (the first is the import, the rest are
// extra dependents).
func resolveGoImport(module, spec string, m *ProjectMap) (string, []string) {
	if module == "" || (spec != module && !strings.HasPrefix(spec, module+"/")) {
		return "", nil // standard library or another module
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(spec, module), "/")
	var pkg []string
	for p := range m.Files {
		if !strings.HasSuffix(p, ".go") {
			continue
		}
		if rel == "" && !strings.Contains(p, "/") || rel != "" && (p == rel+".go" || strings.HasPrefix(p, rel+"/")) {
			pkg = append(pkg, p)
		}
	}
	if len(pkg) == 0 {
		return "", nil
	}
	sort.Strings(pkg)
	return pkg[0], pkg[1:]
}

// goModule reads the module path from go.mod, the base of internal import resolution.
func goModule(root string) string {
	b, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "module" {
			return f[1]
		}
	}
	return ""
}

// resolveRustImport maps use/mod paths to a file in the map: crate::, self::, super:: and mod names.
func resolveRustImport(file, spec string, m *ProjectMap) (string, bool) {
	dir := path.Dir(file)
	rel := spec
	up := 0
	for strings.HasPrefix(rel, "super::") {
		up++
		rel = strings.TrimPrefix(rel, "super::")
	}
	switch {
	case strings.HasPrefix(rel, "crate::"):
		dir, rel = "src", strings.TrimPrefix(rel, "crate::")
	case strings.HasPrefix(rel, "self::"):
		rel = strings.TrimPrefix(rel, "self::")
	case !strings.Contains(rel, "::"):
		// mod name: same directory
	default:
		return "", false // external crate
	}
	for i := 0; i < up; i++ {
		dir = path.Dir(dir)
	}
	if dir == "." {
		dir = "src"
	}
	parts := strings.Split(rel, "::")
	for len(parts) > 0 { // use crate::greet::hi names an item: the longest module prefix is the file
		base := path.Join(dir, strings.Join(parts, "/"))
		for _, c := range []string{base + ".rs", path.Join(base, "mod.rs"), path.Join(base, "lib.rs")} {
			if _, ok := m.Files[c]; ok {
				return c, true
			}
		}
		parts = parts[:len(parts)-1]
	}
	return "", false
}
