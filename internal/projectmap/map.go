package projectmap

// Project map v0: files, exports, imports, dependents, hotspots — generated from code only.

import (
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type FileInfo struct {
	Path       string   `json:"path"`
	Lang       string   `json:"lang"`
	Lines      int      `json:"lines"`
	Bytes      int64    `json:"bytes"`
	Exports    []string `json:"exports"`
	Imports    []string `json:"imports"`
	Dependents []string `json:"dependents"`
}

type DirInfo struct {
	Path    string   `json:"path"`
	Files   []string `json:"files"`
	Subdirs []string `json:"subdirs"`
	Lines   int      `json:"lines"`
}

type Hotspot struct {
	Path   string `json:"path"`
	Lines  int    `json:"lines"`
	Reason string `json:"reason"`
}

type ProjectMap struct {
	GeneratedAt int64                `json:"generatedAt"`
	Root        string               `json:"root"`
	Files       map[string]*FileInfo `json:"files"`
	Dirs        map[string]*DirInfo  `json:"dirs"`
	Hotspots    []Hotspot            `json:"hotspots"`
}

var IgnoredDirs = map[string]bool{"node_modules": true, ".git": true, ".infuse": true, "dist": true, "build": true, "out": true, ".next": true, "coverage": true, "__pycache__": true, ".venv": true, "venv": true, "target": true, "vendor": true, "bin": true, "obj": true}

var langByExt = map[string]string{
	".ts": "ts", ".tsx": "ts", ".mts": "ts", ".cts": "ts",
	".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".py": "py", ".go": "go", ".rs": "rs", ".java": "java", ".kt": "kotlin", ".cs": "cs",
	".rb": "rb", ".php": "php", ".css": "css", ".scss": "css", ".html": "html",
	".json": "json", ".md": "md", ".yml": "yaml", ".yaml": "yaml", ".toml": "toml", ".sh": "sh",
}

const hotspotLines = 800

var (
	reExpJs1   = regexp.MustCompile(`(?m)^export\s+(?:default\s+)?(?:async\s+)?(?:function\*?|class|const|let|var|interface|type|enum|abstract\s+class)\s+([A-Za-z_$][\w$]*)`)
	reExpJs2   = regexp.MustCompile(`(?m)^export\s*\{([^}]*)\}`)
	reExpJsDef = regexp.MustCompile(`(?m)^export\s+default\b`)
	reExpCjs   = regexp.MustCompile(`(?m)^module\.exports\s*=\s*\{([^}]*)\}`)
	reExpPy    = regexp.MustCompile(`(?m)^(?:def|class)\s+([A-Za-z_]\w*)`)
	reExpGo1   = regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)`)
	reExpGo2   = regexp.MustCompile(`(?m)^type\s+([A-Z]\w*)`)
	reExpRs    = regexp.MustCompile(`(?m)^\s*pub\s+(?:async\s+)?(?:fn|struct|enum|trait|type|mod|const|static)\s+([A-Za-z_]\w*)`)
	reExpCs    = regexp.MustCompile(`(?m)^\s*(?:public|internal|open|data|sealed)\s+(?:\w+\s+)*?(?:class|interface|record|enum|struct|object)\s+([A-Za-z_]\w*)`)

	reImpJs1 = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]`)
	reImpJs2 = regexp.MustCompile(`\brequire\(\s*['"]([^'"]+)['"]\s*\)`)
	reImpJs3 = regexp.MustCompile(`\bimport\(\s*['"]([^'"]+)['"]\s*\)`)
	reImpPy1 = regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import`)
	reImpPy2 = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)`)
	reImpGo  = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*$`)
	reImpRs1 = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?use\s+([\w:]+)`)
	reImpRs2 = regexp.MustCompile(`(?m)^\s*mod\s+(\w+)\s*;`)
)

func BuildMap(root string) *ProjectMap {
	m := &ProjectMap{GeneratedAt: time.Now().UnixMilli(), Root: root, Files: map[string]*FileInfo{}, Dirs: map[string]*DirInfo{}}
	walkDir(root, root, m)
	resolveImports(root, m)
	for _, f := range m.Files {
		if f.Lines >= hotspotLines {
			m.Hotspots = append(m.Hotspots, Hotspot{f.Path, f.Lines, fmt.Sprintf("file holds %d lines; responsibilities are likely accumulating here", f.Lines)})
		}
	}
	sort.Slice(m.Hotspots, func(i, j int) bool { return m.Hotspots[i].Lines > m.Hotspots[j].Lines })
	return m
}

func walkDir(root, dir string, m *ProjectMap) int {
	rel, _ := filepath.Rel(root, dir)
	rel = core.ToPosix(rel)
	if rel == "" {
		rel = "."
	}
	info := &DirInfo{Path: rel, Files: []string{}, Subdirs: []string{}}
	m.Dirs[rel] = info
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(dir, name)
		if e.IsDir() {
			if IgnoredDirs[name] || strings.HasPrefix(name, ".") {
				continue
			}
			info.Subdirs = append(info.Subdirs, name)
			info.Lines += walkDir(root, full, m)
			continue
		}
		lang, ok := langByExt[strings.ToLower(filepath.Ext(name))]
		if !ok {
			continue
		}
		st, err := e.Info()
		if err != nil || st.Size() > 2_000_000 {
			continue
		}
		b, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		text := string(b)
		relFile, _ := filepath.Rel(root, full)
		relFile = core.ToPosix(relFile)
		lines := 0
		if len(text) > 0 {
			lines = strings.Count(text, "\n") + 1
		}
		fi := &FileInfo{Path: relFile, Lang: lang, Lines: lines, Bytes: st.Size(), Exports: extractExports(text, lang), Imports: extractImports(text, lang), Dependents: []string{}}
		m.Files[relFile] = fi
		info.Files = append(info.Files, name)
		info.Lines += lines
	}
	return info.Lines
}

func addAll(set map[string]bool, re *regexp.Regexp, text string) {
	for _, mm := range re.FindAllStringSubmatch(text, -1) {
		if len(mm) > 1 && mm[1] != "" {
			set[mm[1]] = true
		}
	}
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func extractExports(text, lang string) []string {
	set := map[string]bool{}
	switch lang {
	case "ts", "js":
		addAll(set, reExpJs1, text)
		for _, mm := range reExpJs2.FindAllStringSubmatch(text, -1) {
			for _, n := range strings.Split(mm[1], ",") {
				parts := regexp.MustCompile(`\s+as\s+`).Split(strings.TrimSpace(n), -1)
				if t := strings.TrimSpace(parts[len(parts)-1]); t != "" {
					set[t] = true
				}
			}
		}
		if reExpJsDef.MatchString(text) {
			set["default"] = true
		}
		for _, mm := range reExpCjs.FindAllStringSubmatch(text, -1) {
			for _, n := range strings.Split(mm[1], ",") {
				if t := strings.TrimSpace(strings.SplitN(strings.TrimSpace(n), ":", 2)[0]); t != "" {
					set[t] = true
				}
			}
		}
	case "py":
		for _, mm := range reExpPy.FindAllStringSubmatch(text, -1) {
			if !strings.HasPrefix(mm[1], "_") {
				set[mm[1]] = true
			}
		}
	case "go":
		addAll(set, reExpGo1, text)
		addAll(set, reExpGo2, text)
	case "rs":
		addAll(set, reExpRs, text)
	case "cs", "java", "kotlin":
		addAll(set, reExpCs, text)
	}
	return keys(set)
}

func extractImports(text, lang string) []string {
	set := map[string]bool{}
	switch lang {
	case "ts", "js":
		addAll(set, reImpJs1, text)
		addAll(set, reImpJs2, text)
		addAll(set, reImpJs3, text)
	case "py":
		addAll(set, reImpPy1, text)
		addAll(set, reImpPy2, text)
	case "go":
		addAll(set, reImpGo, text)
	case "rs":
		addAll(set, reImpRs1, text)
		addAll(set, reImpRs2, text)
	}
	return keys(set)
}

var tsExts = []string{".ts", ".tsx", ".mts", ".js", ".jsx", ".mjs", ".cjs", "/index.ts", "/index.tsx", "/index.js"}

func resolveImports(root string, m *ProjectMap) {
	for _, f := range m.Files {
		for i, spec := range f.Imports {
			switch f.Lang {
			case "ts", "js":
				if !strings.HasPrefix(spec, ".") && !strings.HasPrefix(spec, "/") {
					continue
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
						break
					}
				}
			case "py":
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
						break
					}
				}
			}
		}
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

// DescribeDir: short, agent-facing description of one directory — what is infused into `ls` output.
func DescribeDir(m *ProjectMap, relDir string) string {
	if relDir == "" {
		relDir = "."
	}
	d, ok := m.Dirs[relDir]
	if !ok {
		return ""
	}
	name := d.Path
	if name == "." {
		name = "project root"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[infuse] %s: %d files, %d dirs, %d lines", name, len(d.Files), len(d.Subdirs), d.Lines)
	for _, fn := range d.Files {
		key := fn
		if d.Path != "." {
			key = d.Path + "/" + fn
		}
		f, ok := m.Files[key]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n  %s (%dl)", fn, f.Lines)
		if len(f.Exports) > 0 {
			ex := f.Exports
			more := ""
			if len(ex) > 8 {
				ex, more = ex[:8], "…"
			}
			fmt.Fprintf(&b, " exports %s%s", strings.Join(ex, ", "), more)
		}
		if len(f.Dependents) > 0 {
			fmt.Fprintf(&b, " ← used by %d", len(f.Dependents))
		}
		if f.Lines >= hotspotLines {
			b.WriteString(" ⚠ hotspot")
		}
	}
	for _, s := range d.Subdirs {
		key := s
		if d.Path != "." {
			key = d.Path + "/" + s
		}
		if sd, ok := m.Dirs[key]; ok {
			fmt.Fprintf(&b, "\n  %s/ (%dl, %d files)", s, sd.Lines, len(sd.Files))
		}
	}
	return b.String()
}

// DescribeDependents: who depends on a file — infused when the agent writes it (English).
func DescribeDependents(m *ProjectMap, relFile string) string {
	return DependentsMsg(m, relFile).String()
}

// DependentsMsg: who depends on a file, as a message (empty when nobody does).
func DependentsMsg(m *ProjectMap, relFile string) core.Msg {
	f, ok := m.Files[relFile]
	if !ok || len(f.Dependents) == 0 {
		return core.Msg{}
	}
	deps := f.Dependents
	more := ""
	if len(deps) > 12 {
		deps, more = deps[:12], "…"
	}
	return core.M("map.dependents", "file", relFile, "count", len(f.Dependents), "files", strings.Join(deps, ", ")+more)
}

func MapFile(root string) string { return filepath.Join(root, core.InfuseDirName, "map", "map.json") }

func WriteMap(root string, m *ProjectMap) {
	_ = core.WriteJSON(MapFile(root), m)
	var md strings.Builder
	fmt.Fprintf(&md, "# Project map (generated %s)\n\n", time.UnixMilli(m.GeneratedAt).Format(time.RFC3339))
	dirs := make([]string, 0, len(m.Dirs))
	for k := range m.Dirs {
		dirs = append(dirs, k)
	}
	sort.Strings(dirs)
	for _, d := range dirs {
		md.WriteString(DescribeDir(m, d) + "\n")
	}
	if len(m.Hotspots) > 0 {
		md.WriteString("\n## Hotspots\n")
		for _, h := range m.Hotspots {
			fmt.Fprintf(&md, "- %s: %s\n", h.Path, h.Reason)
		}
	}
	_ = os.WriteFile(filepath.Join(root, core.InfuseDirName, "map", "MAP.md"), []byte(md.String()), 0o644)
}

func LoadMap(root string) *ProjectMap {
	var m ProjectMap
	if core.ReadJSON(MapFile(root), &m) != nil {
		return nil
	}
	return &m
}

func LoadOrBuildMap(root string) *ProjectMap {
	if m := LoadMap(root); m != nil {
		return m
	}
	m := BuildMap(root)
	WriteMap(root, m)
	return m
}
