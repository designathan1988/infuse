package agent

// Architecture, enforced: on every write infuse checks the new content against rules learned from the
// project (where files live, the design tokens, the store) and the limits established tools use, and
// refuses what makes the architecture worse — with the way out. Like ArchUnit's FreezingArchRule and
// dependency-cruiser's baseline, only what the write adds is judged: existing debt does not block work,
// it just cannot grow. No model judges anything here; every check is deterministic.
//
// Sources of the limits (read 2026-10-31):
//   function length: ESLint max-lines-per-function 50, golangci funlen 60 lines
//     https://eslint.org/docs/latest/rules/max-lines-per-function · https://golangci-lint.run/docs/linters/configuration/
//   cyclomatic complexity: ESLint complexity 20, golangci cyclop 10, gocyclo recommends 10–20
//     https://eslint.org/docs/latest/rules/complexity
//   nesting: golangci nestif 5
//   duplication: jscpd minLines 5 · https://github.com/kucherenko/jscpd
//   colours outside tokens: stylelint color-no-hex / declaration-strict-value
//     https://stylelint.io/user-guide/rules/ · https://github.com/AndyOGo/stylelint-declaration-strict-value
//   inline styles: html-validate no-inline-style, react/forbid-dom-props
//   one store, no mutation elsewhere: https://redux.js.org/style-guide/
//   table-driven methods: Code Complete 2 ch. 18; Sonar S1479 max-switch-cases
//   JSON Schema 2020-12: https://json-schema.org/draft/2020-12

import (
	"infuse/internal/fsx"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArchRules is .infuse/architecture.json: learned from the project, adjustable by the user, never by the agent.
type ArchRules struct {
	store.Versioned
	Version     int      `json:"version"`
	TopDirs     []string `json:"topDirs"`    // folders at the root that hold the project's work
	RootExts    []string `json:"rootExts"`   // code extensions that already live at the root
	TokenFiles  []string `json:"tokenFiles"` // where colours and sizes are defined (CSS custom properties)
	StoreFiles  []string `json:"storeFiles"` // the state store(s)
	SchemaDirs  []string `json:"schemaDirs"` // where JSON schemas live
	FuncLines   int      `json:"funcLines"`  // longest function
	Cyclomatic  int      `json:"cyclomatic"` // most branches in a function
	Nesting     int      `json:"nesting"`    // deepest nesting in a function
	DupLines    int      `json:"dupLines"`   // lines repeated from another file that count as a copy
	TableArms   int      `json:"tableArms"`  // literal branches that should be a table
	Disabled    []string `json:"disabled,omitempty"`
	Description string   `json:"description"`
}

func archFile(root string) string { return filepath.Join(root, infuseDirName, "architecture.json") }

// wellKnownDirs are folders any project may start (sources, tests, docs, assets…).
var wellKnownDirs = map[string]bool{"src": true, "lib": true, "app": true, "apps": true, "packages": true, "pkg": true, "internal": true, "cmd": true, "test": true, "tests": true, "e2e": true, "spec": true, "docs": true, "doc": true, "scripts": true, "tools": true, "public": true, "static": true, "assets": true, "styles": true, "components": true, "config": true, "schemas": true, "data": true, "ui": true, "server": true, "client": true, "api": true, "web": true, "examples": true, "migrations": true}

// loadArchRules reads the project's rules, learning them the first time.
func loadArchRules(root string) *ArchRules {
	var r ArchRules
	if readJSON(archFile(root), &r) == nil && r.Version > 0 {
		_, _ = r.Upgrade(nil)
		return &r
	}
	r = learnArchitecture(root)
	r.SchemaVersion = store.Version
	_ = writeJSON(archFile(root), r)
	return &r
}

// learnArchitecture reads the structure the project already has.
func learnArchitecture(root string) ArchRules {
	r := ArchRules{Version: 1, FuncLines: 60, Cyclomatic: 15, Nesting: 5, DupLines: 6, TableArms: 6,
		Description: "Learned from the project by infuse. Folders and token/state files detected; limits from the official tools (ESLint, golangci, jscpd, stylelint). Only what a change adds is judged."}
	entries, _ := os.ReadDir(root)
	exts := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if !fsx.SkipDir(name, fsx.Project) {
				r.TopDirs = append(r.TopDirs, name)
			}
			continue
		}
		if ext := strings.ToLower(filepath.Ext(name)); codeExts[ext] {
			exts[ext] = true
		}
	}
	for e := range exts {
		r.RootExts = append(r.RootExts, e)
	}
	sort.Strings(r.RootExts)
	walkProjectFiles(root, func(rel, abs string, size int64) {
		ext := strings.ToLower(filepath.Ext(rel))
		if size > 400<<10 {
			return
		}
		switch {
		case ext == ".css" || ext == ".scss" || ext == ".less":
			if b, err := os.ReadFile(abs); err == nil && strings.Count(string(b), "--") >= 5 && reCSSVarDecl.MatchString(string(b)) {
				r.TokenFiles = append(r.TokenFiles, rel)
			}
		case jsLikeExts[ext]:
			if b, err := os.ReadFile(abs); err == nil && reStoreCreate.Match(b) {
				r.StoreFiles = append(r.StoreFiles, rel)
			}
		case strings.HasSuffix(rel, ".schema.json"):
			if d := filepath.ToSlash(filepath.Dir(rel)); !contains(r.SchemaDirs, d) {
				r.SchemaDirs = append(r.SchemaDirs, d)
			}
		}
	})
	return r
}

// walkProjectFiles visits the project's own files (no dependencies, caches, build output or .infuse).
func walkProjectFiles(root string, f func(rel, abs string, size int64)) {
	n := 0
	_ = fsx.Walk(root, fsx.Project, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if n++; n > 6000 {
			return filepath.SkipAll
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		f(filepath.ToSlash(rel), p, info.Size())
		return nil
	})
}

// treeShape tells whether the root has code with this extension now, and which top folders hold code.
func treeShape(work, ext string) (bool, []string) {
	rootHas := false
	var dirs []string
	entries, _ := os.ReadDir(work)
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			rootHas = rootHas || strings.EqualFold(filepath.Ext(name), ext)
			continue
		}
		if fsx.SkipDir(name, fsx.Project) {
			continue
		}
		hasCode := false
		_ = fsx.Walk(filepath.Join(work, name), fsx.Snapshot, func(p string, d os.DirEntry, err error) error {
			if err != nil || hasCode {
				return filepath.SkipAll
			}
			if !d.IsDir() && codeExts[strings.ToLower(filepath.Ext(p))] {
				hasCode = true
			}
			return nil
		})
		if hasCode {
			dirs = append(dirs, name+"/")
		}
	}
	return rootHas, dirs
}

func (r *ArchRules) on(rule string) bool { return !contains(r.Disabled, rule) }

// archCheck returns why a write breaks the project's architecture (empty when it does not).
// rel is relative to the project; before is "" for a new file.
func archCheck(root, work, rel, before, after string) Msg {
	r := loadArchRules(root)
	ext := strings.ToLower(filepath.Ext(rel))
	isNew := before == ""
	checks := []func() Msg{
		func() Msg { return archPlacement(r, work, rel, ext, isNew) },
		func() Msg { return archFunctions(r, rel, ext, before, after) },
		func() Msg { return archTables(r, ext, before, after) },
		func() Msg { return archStyles(r, work, rel, ext, before, after) },
		func() Msg { return archState(r, rel, ext, before, after) },
		func() Msg { return archConstants(r, work, rel, ext, before, after) },
		func() Msg { return archDuplicates(r, work, rel, ext, before, after) },
		func() Msg { return archCycles(r, work, rel, ext, before, after) },
		func() Msg { return archJSON(r, work, rel, ext, after) },
	}
	for _, c := range checks {
		if why := c(); !why.Empty() {
			return why
		}
	}
	return Msg{}
}

// archPlacement: a new file goes inside the structure the project has, not loose at the root or in a
// folder invented for it.
// rootEntryName recognizes the files an application normally keeps at the root even after its code
// moved into folders: entry points, static entry files and tool configuration.
func rootEntryName(name string) bool {
	low := strings.ToLower(name)
	switch low {
	case "index.html", "index.css", "style.css", "styles.css", "main.js", "main.ts",
		"server.js", "server.ts", "app.js", "app.ts", "bootstrap.js", "bootstrap.ts",
		"tsconfig.json", "vite.config.js", "vite.config.ts", "webpack.config.js",
		"next.config.js", "next.config.ts", "tailwind.config.js", "tailwind.config.ts",
		"postcss.config.js", "eslint.config.js", "jest.config.js", "vitest.config.js":
		return true
	}
	return strings.HasSuffix(low, ".config.js") || strings.HasSuffix(low, ".config.ts")
}

func archPlacement(r *ArchRules, work, rel, ext string, isNew bool) Msg {
	if !isNew || !r.on("placement") {
		return Msg{}
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 1 { // at the root: loose only when the project already keeps its code in folders
		if !codeExts[ext] || rootEntryName(parts[0]) || contains(r.RootExts, ext) {
			return Msg{}
		}
		rootHas, dirs := treeShape(work, ext)
		if rootHas || len(dirs) == 0 {
			return Msg{} // the root already holds this kind of code, or the project has no folders of code yet
		}
		return M("arch.loose_root", "file", rel, "dirs", strings.Join(dirs, ", "))
	}
	top := parts[0]
	if contains(r.TopDirs, top) || wellKnownDirs[top] || strings.HasPrefix(top, ".") {
		return Msg{}
	}
	if _, err := os.Stat(filepath.Join(work, top)); err == nil {
		return Msg{} // a folder that exists (made by the user or the project)
	}
	return M("arch.new_folder", "dir", top, "dirs", strings.Join(r.TopDirs, ", "))
}
