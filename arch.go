package main

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
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ArchRules is .infuse/architecture.json: learned from the project, adjustable by the user, never by the agent.
type ArchRules struct {
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
		return &r
	}
	r = learnArchitecture(root)
	_ = writeJSON(archFile(root), r)
	return &r
}

// learnArchitecture reads the structure the project already has.
func learnArchitecture(root string) ArchRules {
	r := ArchRules{Version: 1, FuncLines: 60, Cyclomatic: 15, Nesting: 5, DupLines: 6, TableArms: 6,
		Description: "Aprendido do projeto pelo infuse. Pastas e arquivos de tokens/estado detectados; limites das ferramentas oficiais (ESLint, golangci, jscpd, stylelint). Só o que uma alteração acrescenta é julgado."}
	entries, _ := os.ReadDir(root)
	exts := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if !strings.HasPrefix(name, ".") && !snapSkipDirs[name] && !ignoredDirs[name] {
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
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && (snapSkipDirs[d.Name()] || ignoredDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
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
		if strings.HasPrefix(name, ".") || snapSkipDirs[name] || ignoredDirs[name] {
			continue
		}
		hasCode := false
		_ = filepath.WalkDir(filepath.Join(work, name), func(p string, d os.DirEntry, err error) error {
			if err != nil || hasCode {
				return filepath.SkipAll
			}
			if d.IsDir() && (snapSkipDirs[d.Name()] || ignoredDirs[d.Name()]) {
				return filepath.SkipDir
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

// archCheck returns why a write breaks the project's architecture ("" when it does not).
// rel is relative to the project; before is "" for a new file.
func archCheck(root, work, rel, before, after string) string {
	r := loadArchRules(root)
	ext := strings.ToLower(filepath.Ext(rel))
	isNew := before == ""
	checks := []func() string{
		func() string { return archPlacement(r, work, rel, ext, isNew) },
		func() string { return archFunctions(r, rel, ext, before, after) },
		func() string { return archTables(r, ext, before, after) },
		func() string { return archStyles(r, work, rel, ext, before, after) },
		func() string { return archState(r, rel, ext, before, after) },
		func() string { return archConstants(r, work, rel, ext, before, after) },
		func() string { return archDuplicates(r, work, rel, ext, before, after) },
		func() string { return archCycles(r, work, rel, ext, before, after) },
		func() string { return archJSON(r, work, rel, ext, after) },
	}
	for _, c := range checks {
		if why := c(); why != "" {
			return "[infuse] Arquitetura: " + why
		}
	}
	return ""
}

// archPlacement: a new file goes inside the structure the project has, not loose at the root or in a
// folder invented for it.
func archPlacement(r *ArchRules, work, rel, ext string, isNew bool) string {
	if !isNew || !r.on("placement") {
		return ""
	}
	parts := strings.Split(rel, "/")
	if len(parts) == 1 { // at the root: loose only when the project already keeps its code in folders
		if !codeExts[ext] || contains(r.RootExts, ext) {
			return ""
		}
		rootHas, dirs := treeShape(work, ext)
		if rootHas || len(dirs) == 0 {
			return "" // the root already holds this kind of code, or the project has no folders of code yet
		}
		return "arquivo de código solto na raiz (" + rel + "), num projeto que organiza o código em pastas. Coloque-o na pasta da sua responsabilidade: " + strings.Join(dirs, ", ") + "."
	}
	top := parts[0]
	if contains(r.TopDirs, top) || wellKnownDirs[top] || strings.HasPrefix(top, ".") {
		return ""
	}
	if _, err := os.Stat(filepath.Join(work, top)); err == nil {
		return "" // a folder that exists (made by the user or the project)
	}
	return "pasta nova na raiz (" + top + "/) sem lugar na estrutura do projeto. Use uma das pastas existentes (" + strings.Join(r.TopDirs, ", ") + ") ou um nome de papel conhecido (src, lib, test, docs, scripts, assets…)."
}
