package projectmap

// Import resolution: map a dependency to the file it names.

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"infuse/internal/core"
	"infuse/internal/lang"
)

var tsExts = []string{".ts", ".tsx", ".mts", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte", "/index.ts", "/index.tsx", "/index.js"}

// resolver holds the indexes a batch of imports is resolved against: without them every import walks
// every file, and a large project pays that product on each build.
type resolver struct {
	root    string
	m       *ProjectMap
	module  string
	goPkgs  map[string][]string // folder → its non-test Go files
	nsFiles map[string][]string // namespace or package → files declaring it
	byName  map[string][]string // base name → C/C++ files, for includes given without their folder
}

func newResolver(root string, m *ProjectMap) *resolver {
	r := &resolver{root: root, m: m, module: goModule(root), goPkgs: map[string][]string{}, nsFiles: map[string][]string{}, byName: map[string][]string{}}
	for p, f := range m.Files {
		r.index(p, f)
	}
	for _, list := range []map[string][]string{r.goPkgs, r.nsFiles, r.byName} {
		for _, v := range list {
			sort.Strings(v)
		}
	}
	return r
}

func (r *resolver) index(p string, f *FileInfo) {
	switch {
	case f.Lang == "go" && !lang.IsTestPath(p):
		r.goPkgs[path.Dir(p)] = append(r.goPkgs[path.Dir(p)], p)
	case f.Namespace != "":
		r.nsFiles[f.Namespace] = append(r.nsFiles[f.Namespace], p)
	case f.Lang == "c" || f.Lang == "cpp":
		r.byName[path.Base(p)] = append(r.byName[path.Base(p)], p)
	}
}

func resolveImports(root string, m *ProjectMap) {
	r := newResolver(root, m)
	for _, f := range m.Files {
		r.resolveFile(f)
	}
	for _, f := range m.Files {
		for _, imp := range f.Imports {
			if t, ok := m.Files[imp]; ok && t != f {
				t.Dependents = append(t.Dependents, f.Path)
			}
		}
	}
	for _, f := range m.Files {
		sort.Strings(f.Dependents)
	}
}

// resolveFile rewrites one file's imports into the mapped files they name.
func (r *resolver) resolveFile(f *FileInfo) {
	var extra []string
	for i, spec := range f.Imports {
		switch f.Lang {
		case "ts", "js", "vue", "svelte":
			resolveTSImport(r.root, f, i, spec, r.m)
		case "py":
			resolvePyImport(r.root, f, i, spec, r.m)
		case "go":
			if rel, more := r.resolveGoImport(spec); rel != "" {
				f.Imports[i] = rel
				extra = append(extra, more...)
			}
		case "rs":
			if rel, ok := resolveRustImport(f.Path, spec, r.m); ok {
				f.Imports[i] = rel
			}
		case "cs":
			r.resolveNamespacedImport(f, i, spec)
		case "java", "kotlin":
			extra = append(extra, r.resolveJvmImport(f, i, spec)...)
		case "c", "cpp":
			r.resolveInclude(f, i, spec)
		}
	}
	f.Imports = dedupe(f.Imports, extra...)
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
func (r *resolver) resolveGoImport(spec string) (string, []string) {
	if r.module == "" || (spec != r.module && !strings.HasPrefix(spec, r.module+"/")) {
		return "", nil // standard library or another module
	}
	dir := strings.TrimPrefix(strings.TrimPrefix(spec, r.module), "/")
	if dir == "" {
		dir = "."
	}
	pkg := r.goPkgs[dir]
	if len(pkg) == 0 {
		return "", nil
	}
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

// resolveNamespacedImport maps a C# using to a file that declares that namespace.
func (r *resolver) resolveNamespacedImport(f *FileInfo, i int, spec string) {
	var cands []string
	for ns, files := range r.nsFiles {
		if ns == spec || strings.HasPrefix(ns, spec+".") || strings.HasPrefix(spec, ns+".") {
			for _, p := range files {
				if p != f.Path {
					cands = append(cands, p)
				}
			}
		}
	}
	if len(cands) > 0 {
		sort.Strings(cands)
		f.Imports[i] = cands[0]
	}
}

// resolveJvmImport maps a Java or Kotlin import to the file of the class it names: a.b.C is C.java or
// C.kt in package a.b, a.b.C.member is the same file, and a.b.* is every file of the package (the
// first becomes the import, the others are returned as extras).
func (r *resolver) resolveJvmImport(f *FileInfo, i int, spec string) []string {
	if pkg, wild := strings.CutSuffix(spec, ".*"); wild {
		files := r.nsFiles[pkg]
		if len(files) == 0 {
			return nil
		}
		f.Imports[i] = files[0]
		return files[1:]
	}
	parts := strings.Split(spec, ".")
	for n := len(parts) - 1; n > 0; n-- {
		ns, class := strings.Join(parts[:n], "."), parts[n]
		for _, p := range r.nsFiles[ns] {
			base := path.Base(p)
			if strings.TrimSuffix(base, path.Ext(base)) == class {
				f.Imports[i] = p
				return nil
			}
		}
	}
	if files := r.nsFiles[strings.Join(parts[:len(parts)-1], ".")]; len(files) > 0 {
		f.Imports[i] = files[0] // a Kotlin top-level function: the package is all that is known
	}
	return nil
}

// resolveInclude maps #include "x.h": beside the file, from the project's usual include folders, then
// by a name only one file carries.
func (r *resolver) resolveInclude(f *FileInfo, i int, spec string) {
	for _, dir := range []string{path.Dir(f.Path), ".", "include", "src"} {
		if c := path.Join(dir, spec); r.m.Files[c] != nil {
			f.Imports[i] = c
			return
		}
	}
	if list := r.byName[path.Base(spec)]; len(list) == 1 {
		f.Imports[i] = list[0]
	}
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
