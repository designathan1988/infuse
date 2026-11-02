package projectmap

// Project map: files, exports, imports, dependents, hotspots, and the levels above them (features,
// data owners, real examples, intended vs. actual) — generated from code only.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type FileInfo struct {
	Path       string   `json:"path"`
	Lang       string   `json:"lang"`
	Lines      int      `json:"lines"`
	Bytes      int64    `json:"bytes"`
	Namespace  string   `json:"namespace,omitempty"` // C# namespace / Java-Kotlin package
	Exports    []string `json:"exports"`
	Imports    []string `json:"imports"`
	Dependents []string `json:"dependents"`

	scan *fileScan // what a full build keeps while it computes the levels; never stored
}

type DirInfo struct {
	Path     string   `json:"path"`
	Files    []string `json:"files"`
	Subdirs  []string `json:"subdirs"`
	Lines    int      `json:"lines"`
	Intended bool     `json:"intended,omitempty"` // declared by the base, no code there yet
	Role     string   `json:"role,omitempty"`     // what the base says lives in this folder
}

type Hotspot struct {
	Path   string `json:"path"`
	Lines  int    `json:"lines"`
	Reason string `json:"reason"`
}

// ProjectMap is the project as it is. Root is where this copy was read from, never stored: the map
// is versioned, and a versioned file carries no machine's path.
type ProjectMap struct {
	GeneratedAt  int64                `json:"generatedAt"`
	Root         string               `json:"-"`
	HotspotLines int                  `json:"hotspotLines,omitempty"`
	Files        map[string]*FileInfo `json:"files"`
	Dirs         map[string]*DirInfo  `json:"dirs"`
	Hotspots     []Hotspot            `json:"hotspots"`
	Features     []Feature            `json:"features,omitempty"` // feature → where it is split
	Data         []DataOwner          `json:"data,omitempty"`     // data → owner
	Examples     []Example            `json:"examples,omitempty"` // "how X is done here" → real code
	Drift        []Drift              `json:"drift,omitempty"`    // intended vs. actual
	Dead         []DeadExport         `json:"dead,omitempty"`     // exports nothing references

	partial bool // a single folder read for one answer: never stored as the project's map
}

// defaultHotspotLines applies when no base states the project's own limit.
const defaultHotspotLines = 800

// maxScannedFile bounds what the map reads: bigger files are generated data, not code to describe.
const maxScannedFile = 2_000_000

// BuildMap reads the whole project and computes every level of the map.
func BuildMap(root string) *ProjectMap {
	m := &ProjectMap{GeneratedAt: time.Now().UnixMilli(), Root: root, Files: map[string]*FileInfo{}, Dirs: map[string]*DirInfo{}}
	walkDir(root, root, m)
	resolveImports(root, m)
	intended := LoadContract(root)
	applyContract(m, intended)
	computeHotspots(m, intended.MaxFileLines)
	m.Features = computeFeatures(m)
	m.Data = computeData(m)
	m.Examples = computeExamples(m)
	m.Dead = computeDead(m, intended)
	m.Drift = computeDrift(m, intended)
	for _, f := range m.Files {
		f.scan = nil
	}
	return m
}

// computeHotspots lists the files past the project's limit; lockfiles and logs are generated, not
// responsibilities accumulating.
func computeHotspots(m *ProjectMap, limit int) {
	if limit <= 0 {
		limit = defaultHotspotLines
	}
	m.HotspotLines, m.Hotspots = limit, nil
	for _, f := range m.Files {
		if f.Lines >= limit && !isGeneratedNoise(f.Path) {
			m.Hotspots = append(m.Hotspots, Hotspot{f.Path, f.Lines, fmt.Sprintf("file holds %d lines; responsibilities are likely accumulating here", f.Lines)})
		}
	}
	sort.Slice(m.Hotspots, func(i, j int) bool {
		if m.Hotspots[i].Lines != m.Hotspots[j].Lines {
			return m.Hotspots[i].Lines > m.Hotspots[j].Lines
		}
		return m.Hotspots[i].Path < m.Hotspots[j].Path
	})
}

var lockfiles = map[string]bool{
	"package-lock.json": true, "npm-shrinkwrap.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"bun.lock": true, "composer.lock": true, "cargo.lock": true, "poetry.lock": true, "uv.lock": true,
	"pipfile.lock": true, "gemfile.lock": true, "go.sum": true, "packages.lock.json": true,
}

// isGeneratedNoise recognizes lockfiles, logs and minified bundles: long, and nobody's responsibility.
func isGeneratedNoise(rel string) bool {
	low := strings.ToLower(rel)
	name := filepath.Base(filepath.FromSlash(low))
	if lockfiles[name] || strings.HasSuffix(name, ".lock") || strings.HasSuffix(name, ".min.js") || strings.HasSuffix(name, ".min.css") {
		return true
	}
	if strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".jsonl") || strings.HasPrefix(name, "changelog") {
		return true
	}
	return strings.HasPrefix(low, "logs/") || strings.Contains(low, "/logs/")
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
			if !fsx.SkipDir(name, fsx.Project) {
				info.Subdirs = append(info.Subdirs, name)
				info.Lines += walkDir(root, full, m)
			}
			continue
		}
		if f := readFileInfo(root, full, true); f != nil {
			m.Files[f.Path] = f
			info.Files = append(info.Files, name)
			info.Lines += f.Lines
		}
	}
	return info.Lines
}

// readFileInfo reads one mapped file (nil when the map does not cover it); withScan keeps what the
// full build needs for the levels.
func readFileInfo(root, full string, withScan bool) *FileInfo {
	lang, ok := langOf(filepath.Base(full))
	if !ok {
		return nil
	}
	st, err := os.Stat(full)
	if err != nil || st.IsDir() || st.Size() > maxScannedFile {
		return nil
	}
	b, err := os.ReadFile(full)
	if err != nil {
		return nil
	}
	text := string(b)
	relFile, _ := filepath.Rel(root, full)
	relFile = core.ToPosix(relFile)
	lines := 0
	if len(text) > 0 {
		lines = strings.Count(text, "\n") + 1
	}
	f := &FileInfo{Path: relFile, Lang: lang, Lines: lines, Bytes: st.Size(), Namespace: namespaceOf(text, lang), Exports: extractExports(text, lang, relFile), Imports: extractImports(text, lang), Dependents: []string{}}
	if withScan {
		f.scan = scanFile(f, text)
	}
	return f
}

// DescribeDependents: who depends on a file — infused when the agent writes it (English).
func DescribeDependents(m *ProjectMap, relFile string) string {
	return DependentsMsg(m, relFile).String()
}

// DependentsMsg: who depends on a file, as a message (empty when nobody does).
func DependentsMsg(m *ProjectMap, relFile string) core.Msg {
	return UntouchedDependentsMsg(m, relFile, nil)
}

// UntouchedDependentsMsg: who depends on a file and was not touched yet — the files the change may
// still have to reach (empty when none is left).
func UntouchedDependentsMsg(m *ProjectMap, relFile string, touched map[string]bool) core.Msg {
	f, ok := m.Files[relFile]
	if !ok {
		return core.Msg{}
	}
	deps := make([]string, 0, len(f.Dependents))
	for _, d := range f.Dependents {
		if !touched[d] {
			deps = append(deps, d)
		}
	}
	if len(deps) == 0 {
		return core.Msg{}
	}
	shown, more := deps, ""
	if len(shown) > 12 {
		shown, more = shown[:12], "…"
	}
	return core.M("map.dependents", "file", relFile, "count", len(deps), "files", strings.Join(shown, ", ")+more)
}
