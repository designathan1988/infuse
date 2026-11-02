package projectmap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A feature is found by the stem its files share, and a file with no stem of its own joins the one
// feature that imports it. A folder no import ties to the rest stands apart.
func TestFeatureClustersByStemAndImportGraph(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "api/billing_routes.ts", "import { charge } from '../domain/billing';\nimport { round } from '../domain/rounding';\nexport const r = charge(round(1));\n")
	writeMapFile(t, root, "domain/billing.ts", "export function charge(n: number) { return n; }\n")
	writeMapFile(t, root, "domain/rounding.ts", "export function round(n: number) { return n; }\n")
	writeMapFile(t, root, "legacy/billing_old.ts", "export const old = 1;\n")
	m := BuildMap(root)
	f, ok := FeatureOf(m, "api/billing_routes.ts")
	if !ok {
		t.Fatalf("%+v", m.Features)
	}
	if strings.Join(f.Files, ",") != "api/billing_routes.ts,domain/billing.ts,domain/rounding.ts,legacy/billing_old.ts" {
		t.Fatalf("files: %v", f.Files)
	}
	if strings.Join(f.Unlinked, ",") != "legacy" {
		t.Fatalf("unlinked: %v", f.Unlinked)
	}
}

// A datum with one writer has an owner; one written from two files is stated as shared. A type nobody
// writes is a shape, not data.
func TestDataOwnersAcrossLanguages(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeMapFile(t, root, "core/session.go", "package core\n\ntype Session struct{ Name string }\n\nfunc (s *Session) Rename(n string) { s.Name = n }\n\ntype Shape struct{ X int }\n\nvar Cache = map[string]int{}\n")
	writeMapFile(t, root, "core/fill.go", "package core\n\nfunc Fill() { Cache[\"a\"] = 1 }\n")
	writeMapFile(t, root, "core/drain.go", "package core\n\nfunc Drain() { delete(Cache, \"a\") }\n")
	writeMapFile(t, root, "web/store.js", "export let count = 0;\nexport function up() { count += 1; }\n")
	writeMapFile(t, root, "py/state.py", "ITEMS = []\n\ndef add(x):\n    ITEMS.append(x)\n")
	writeMapFile(t, root, "rs/lib.rs", "pub struct Counter { pub n: u32 }\n\npub fn bump() { let mut c: Counter = make(); c.n += 1; }\n")
	writeMapFile(t, root, "cs/Repo.cs", "public class Repo { public int N; }\nclass User { void F(Repo r) { Repo x = new Repo(); r.N = 1; } }\n")
	m := BuildMap(root)
	rows := map[string]DataOwner{}
	for _, d := range m.Data {
		rows[d.Name] = d
	}
	if r := rows["Session"]; r.Owner != "core/session.go" {
		t.Fatalf("Session: %+v", r)
	}
	if _, ok := rows["Shape"]; ok {
		t.Fatal("a type nothing writes is listed as data")
	}
	if r := rows["Cache"]; r.Owner != "" || len(r.Writers) != 2 {
		t.Fatalf("Cache: %+v", r)
	}
	if r := rows["count"]; r.Owner != "web/store.js" {
		t.Fatalf("count: %+v", r)
	}
	if r := rows["ITEMS"]; r.Owner != "py/state.py" {
		t.Fatalf("ITEMS: %+v", r)
	}
	if r := rows["Counter"]; r.Owner != "rs/lib.rs" {
		t.Fatalf("Counter: %+v", r)
	}
	if r := rows["Repo"]; r.Owner != "cs/Repo.cs" {
		t.Fatalf("Repo: %+v", r)
	}
}

// "How it is done here" points at the shortest real file, at the line.
func TestExamplesPointAtTheShortestRealFile(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "a/long.go", "package a\n\nimport \"net/http\"\n\n// padding\n// padding\n// padding\nfunc H(w http.ResponseWriter, r *http.Request) {}\n")
	writeMapFile(t, root, "b/short.go", "package b\n\nimport \"net/http\"\n\nfunc S(w http.ResponseWriter, r *http.Request) {}\n")
	writeMapFile(t, root, "b/short_test.go", "package b\n\nimport \"testing\"\n\nfunc TestS(t *testing.T) {}\n")
	m := BuildMap(root)
	got := map[string]Example{}
	for _, e := range m.Examples {
		got[e.Op] = e
	}
	if e := got["http-handler"]; e.File != "b/short.go" || e.Line != 5 || !strings.Contains(e.Text, "func S") {
		t.Fatalf("%+v", e)
	}
	if e := got["test"]; e.File != "b/short_test.go" || e.Line != 5 {
		t.Fatalf("%+v", e)
	}
}

// Intended vs. actual reports what the code did to the base, not only what is missing.
func TestDriftFindsUndeclaredFoldersAndLayerViolations(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n")
	if err := WriteContract(root, Contract{
		Layout:  []FolderRole{{Path: "internal/api", Role: "transport"}, {Path: "internal/domain", Role: "domain"}, {Path: "internal/store", Role: "storage"}},
		Folders: []string{"internal/api", "internal/domain", "internal/store"},
		Layers:  []Layer{{Path: "internal/api", MayImport: []string{"internal/domain"}}, {Path: "internal/domain", MayImport: []string{}}},
	}); err != nil {
		t.Fatal(err)
	}
	writeMapFile(t, root, "internal/api/a.go", "package api\n")
	writeMapFile(t, root, "internal/domain/d.go", "package domain\n\nimport \"example.com/app/internal/api\"\n\nvar _ = api.X\n")
	writeMapFile(t, root, "scripts/x.go", "package main\n")
	m := BuildMap(root)
	kinds := map[string]Drift{}
	for _, d := range m.Drift {
		kinds[d.Kind] = d
	}
	if d := kinds["undeclared-folder"]; d.Path != "scripts" {
		t.Fatalf("%+v", m.Drift)
	}
	if d := kinds["layer-violation"]; d.Path != "internal/domain/d.go" || !strings.Contains(d.Detail, "may not import internal/api") {
		t.Fatalf("%+v", m.Drift)
	}
	if d := kinds["intended-only"]; d.Path != "internal/store" {
		t.Fatalf("%+v", m.Drift)
	}
	if out := DescribeDir(m, "internal/domain"); !strings.Contains(out, "drift internal/domain/d.go") {
		t.Fatalf("drift not in the folder's map: %s", out)
	}
}

// Lockfiles and logs are long and nobody's responsibility.
func TestHotspotsLeaveOutLogsAndLockfiles(t *testing.T) {
	root := t.TempDir()
	long := strings.Repeat("x\n", 900)
	writeMapFile(t, root, "package-lock.json", long)
	writeMapFile(t, root, "pnpm-lock.yaml", long)
	writeMapFile(t, root, "logs/run.md", long)
	writeMapFile(t, root, "CHANGELOG.md", long)
	writeMapFile(t, root, "src/god.ts", long)
	m := BuildMap(root)
	if len(m.Hotspots) != 1 || m.Hotspots[0].Path != "src/god.ts" {
		t.Fatalf("%+v", m.Hotspots)
	}
}

// Java and Kotlin imports reach the class's own file; .vue, .svelte, C and C++ are read and linked.
func TestNewLanguagesResolve(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/com/acme/Repo.java", "package com.acme;\npublic class Repo {}\n")
	writeMapFile(t, root, "src/com/acme/Other.java", "package com.acme;\nclass Other {}\n")
	writeMapFile(t, root, "src/com/acme/ui/Screen.kt", "package com.acme.ui\nimport com.acme.Repo\nclass Screen\nfun show() {}\n")
	writeMapFile(t, root, "web/Card.vue", "<template><b/></template>\n<script>\nimport util from './util.js';\nexport default {};\n</script>\n")
	writeMapFile(t, root, "web/Page.svelte", "<script>\nimport Card from './Card.vue';\n</script>\n")
	writeMapFile(t, root, "web/util.js", "export default 1;\n")
	writeMapFile(t, root, "native/lib.h", "typedef struct Box Box;\nint box_size(Box *b);\n")
	writeMapFile(t, root, "native/lib.cpp", "#include \"lib.h\"\nint box_size(Box *b) { return 1; }\n")
	m := BuildMap(root)
	if r := m.Files["src/com/acme/Repo.java"]; r == nil || len(r.Dependents) != 1 || r.Dependents[0] != "src/com/acme/ui/Screen.kt" {
		t.Fatalf("java: %+v", r)
	}
	if s := m.Files["src/com/acme/ui/Screen.kt"]; s == nil || !contains(s.Exports, "show") || !contains(s.Exports, "Screen") {
		t.Fatalf("kotlin: %+v", s)
	}
	if c := m.Files["web/Card.vue"]; c == nil || c.Lang != "vue" || len(c.Dependents) != 1 || !contains(c.Exports, "Card") {
		t.Fatalf("vue: %+v", c)
	}
	if u := m.Files["web/util.js"]; u == nil || len(u.Dependents) != 1 || u.Dependents[0] != "web/Card.vue" {
		t.Fatalf("vue import: %+v", u)
	}
	if h := m.Files["native/lib.h"]; h == nil || len(h.Dependents) != 1 || !contains(h.Exports, "box_size") {
		t.Fatalf("c: %+v", h)
	}
}

// An export nothing mentions is dead; one used in its own file, a test name, an entry point and the
// base's public surface are not.
func TestDeadExportsThroughTheMap(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "go.mod", "module example.com/app\n\ngo 1.22\n")
	writeMapFile(t, root, "core/live.go", "package core\n\nfunc Used() {}\n\nfunc Local() {}\n\nvar _ = Local\n\n// Forgotten does nothing, and only this comment mentions it again: Forgotten.\nfunc Forgotten() {}\n\ntype T struct{}\n\nfunc (T) ServeHTTP() {}\n")
	writeMapFile(t, root, "core/caller.go", "package core\n\nfunc call() { Used() }\n")
	writeMapFile(t, root, "src/orphan.ts", "export function nobody() {}\n")
	writeMapFile(t, root, "pkg/lib/lib.go", "package lib\n\nfunc Public() {}\n")
	if err := WriteContract(root, Contract{Folders: []string{"core", "src", "pkg"}, Public: []string{"pkg"}}); err != nil {
		t.Fatal(err)
	}
	m := BuildMap(root)
	dead := map[string]DeadExport{}
	for _, d := range m.Dead {
		dead[d.Symbol] = d
	}
	if _, ok := dead["Forgotten"]; !ok {
		t.Fatalf("%+v", m.Dead)
	}
	if d, ok := dead["nobody"]; !ok || !d.Whole {
		t.Fatalf("%+v", m.Dead)
	}
	for _, alive := range []string{"Used", "Local", "ServeHTTP", "Public"} {
		if _, ok := dead[alive]; ok {
			t.Fatalf("%s is alive: %+v", alive, m.Dead)
		}
	}
}

// The stored map moves when the code moves: no absolute path, no clock in the documents, one document
// per folder, and an orphan document leaves with its folder.
func TestStoredMapIsStableAndPerDirectory(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/a.ts", "export const a = 1;\n")
	writeMapFile(t, root, "gone/b.ts", "export const b = 1;\n")
	WriteMap(root, BuildMap(root))
	raw, err := os.ReadFile(MapFile(root))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), filepath.Base(root)) {
		t.Fatal("the versioned map carries this machine's path")
	}
	for _, f := range []string{DirFile(root, "."), DirFile(root, "src"), DirFile(root, "gone")} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("folder document missing: %v", err)
		}
	}
	before := fileStamp(t, MapFile(root))
	doc, _ := os.ReadFile(mapDocFile(root))
	if strings.Contains(string(doc), "map generated") {
		t.Fatalf("the clock is in MAP.md: %s", doc)
	}
	m := BuildMap(root)
	m.GeneratedAt += 5000
	WriteMap(root, m)
	if fileStamp(t, MapFile(root)) != before {
		t.Fatal("map.json was rewritten with no change in the code")
	}
	_ = os.RemoveAll(filepath.Join(root, "gone"))
	WriteMap(root, BuildMap(root))
	if _, err := os.Stat(DirFile(root, "gone")); err == nil {
		t.Fatal("the document of a removed folder stayed")
	}
}

func fileStamp(t *testing.T, p string) int64 {
	t.Helper()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return st.ModTime().UnixNano()
}

// A file touched by the filter updates the map, MAP.md and its folder's document, and moves its edges
// in the graph, without a rebuild.
func TestUpdateFilesKeepsEveryDocumentFresh(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/a.ts", "export const a = 1;\n")
	writeMapFile(t, root, "src/b.ts", "export const b = 2;\n")
	WriteMap(root, BuildMap(root))
	writeMapFile(t, root, "src/a.ts", "import { b } from './b';\nexport const a = b;\nexport const c = 3;\n")
	writeMapFile(t, root, "src/new.ts", "import { a } from './a';\nexport const n = a;\n")
	_ = os.Remove(filepath.Join(root, "src", "b.ts"))
	UpdateFiles(root, []string{"src/a.ts", "src/new.ts", "src/b.ts"})
	m := LoadMap(root)
	if m.Files["src/b.ts"] != nil || m.Files["src/new.ts"] == nil {
		t.Fatalf("%+v", m.Files)
	}
	if a := m.Files["src/a.ts"]; len(a.Exports) != 2 || len(a.Dependents) != 1 || a.Dependents[0] != "src/new.ts" {
		t.Fatalf("%+v", a)
	}
	full := BuildMap(root)
	if m.Dirs["src"].Lines != full.Dirs["src"].Lines || m.Dirs["."].Lines != full.Dirs["."].Lines {
		t.Fatalf("line totals drifted: %d/%d", m.Dirs["src"].Lines, full.Dirs["src"].Lines)
	}
	doc, _ := os.ReadFile(mapDocFile(root))
	if !strings.Contains(string(doc), "new.ts") || strings.Contains(string(doc), "b.ts") {
		t.Fatalf("MAP.md is stale: %s", doc)
	}
	dir, _ := os.ReadFile(DirFile(root, "src"))
	if !strings.Contains(string(dir), "new.ts") {
		t.Fatalf("the folder document is stale: %s", dir)
	}
}

// A listed folder is described as it is now, whatever happened to it; with no map at all the folder
// is still answered, and nothing partial is stored as the project's map.
func TestRefreshDirWithAndWithoutAMap(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/a.ts", "export const a = 1;\n")
	writeMapFile(t, root, "src/sub/c.ts", "export const c = 1;\n")
	if m := RefreshDir(root, "src"); len(m.Dirs["src"].Files) != 1 || !strings.Contains(DescribeDir(m, "src"), "a.ts") {
		t.Fatalf("%+v", m.Dirs["src"])
	}
	if _, err := os.Stat(MapFile(root)); err == nil {
		t.Fatal("a partial map was stored")
	}
	WriteMap(root, BuildMap(root))
	_ = os.RemoveAll(filepath.Join(root, "src", "sub"))
	writeMapFile(t, root, "src/d.ts", "export const d = 1;\n")
	m := RefreshDir(root, "src")
	if m.Dirs["src/sub"] != nil || m.Files["src/sub/c.ts"] != nil || m.Files["src/d.ts"] == nil {
		t.Fatalf("%+v", m.Dirs)
	}
	if m.Dirs["."].Lines != BuildMap(root).Dirs["."].Lines {
		t.Fatal("line totals drifted")
	}
}

// The generated block never touches what the user wrote, is replaced in place, and leaves with
// nothing else when the feature is turned off.
func TestContextFilesKeepUserContent(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/a.ts", "export const a = 1;\n")
	user := "# My rules\n\nNever do X.\n"
	writeMapFile(t, root, "src/CLAUDE.md", user)
	if ContextFilesEnabled(root) {
		t.Fatal("context files must be off for an existing repository")
	}
	WriteMap(root, BuildMap(root))
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err == nil {
		t.Fatal("a file was generated without being asked for")
	}
	if err := SetContextFiles(root, true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "src", "CLAUDE.md"))
	if !strings.HasPrefix(string(got), user) || !strings.Contains(string(got), "a.ts") {
		t.Fatalf("%s", got)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "GEMINI.md")); err != nil {
		t.Fatal(err)
	}
	writeMapFile(t, root, "src/b.ts", "export const b = 1;\n")
	UpdateFiles(root, []string{"src/b.ts"})
	got, _ = os.ReadFile(filepath.Join(root, "src", "CLAUDE.md"))
	if !strings.Contains(string(got), "b.ts") || strings.Count(string(got), "infuse:begin") != 1 || !strings.HasPrefix(string(got), user) {
		t.Fatalf("%s", got)
	}
	if err := SetContextFiles(root, false); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(root, "src", "CLAUDE.md"))
	if strings.TrimSpace(string(got)) != strings.TrimSpace(user) {
		t.Fatalf("%q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "src", "GEMINI.md")); err == nil {
		t.Fatal("a generated file stayed after off")
	}
}

// An unterminated block is the user's text: nothing after it may be replaced or stripped.
func TestLostEndMarkerProtectsWhatFollows(t *testing.T) {
	file := filepath.Join(t.TempDir(), "CLAUDE.md")
	text := "<!-- infuse:begin map (generated) -->\nold\nmine\n"
	_ = os.WriteFile(file, []byte(text), 0o644)
	if err := UpsertBlock(file, BlockMap, "new"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(file)
	if !strings.HasPrefix(string(got), text[:len(text)-1]) || !strings.Contains(string(got), "new") {
		t.Fatalf("%q", got)
	}
	clean := StripGeneratedBlocks("a\n" + "<!-- infuse:begin map (x) -->\nb\n<!-- infuse:end map -->\nc\n")
	if strings.Contains(clean, "b\n") || !strings.Contains(clean, "c") {
		t.Fatalf("%q", clean)
	}
}
