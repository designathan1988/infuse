package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every message used in the code exists in the catalog, in English and Brazilian Portuguese, and both
// languages use exactly the same named arguments.
func TestCatalogIsCompleteForEveryMessageUsed(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	reUse := regexp.MustCompile(`\bM\("([a-z0-9_.]+)"`)
	used := map[string]string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(f)
		for _, m := range reUse.FindAllStringSubmatch(string(b), -1) {
			used[m[1]] = f
		}
	}
	if len(used) == 0 {
		t.Fatal("no message found in the code")
	}
	catalogMu.RLock()
	defer catalogMu.RUnlock()
	names := func(s string) string {
		var out []string
		for _, m := range rePlaceholder.FindAllStringSubmatch(s, -1) {
			out = append(out, m[1])
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	for id, f := range used {
		langs, ok := catalog[id]
		if !ok {
			t.Errorf("%s (used in %s) is not in the catalog", id, f)
			continue
		}
		en, pt := langs["en"], langs["pt-BR"]
		if en == "" || pt == "" {
			t.Errorf("%s: missing a language (en %q, pt-BR %q)", id, en, pt)
			continue
		}
		if names(en) != names(pt) {
			t.Errorf("%s: arguments differ between languages: en {%s} pt-BR {%s}", id, names(en), names(pt))
		}
	}
}

func TestMessagesRenderPerLanguageWithNestedReasons(t *testing.T) {
	why := M("redo.revert", "file", "src/a.ts")
	m := M("gate.redo", "why", why)
	if en := m.String(); !strings.Contains(en, "a new diagnosis is required (you returned src/a.ts to a content") {
		t.Fatal(en)
	}
	if pt := m.Render("pt-BR"); !strings.Contains(pt, "é preciso um novo diagnóstico (você devolveu src/a.ts a um conteúdo") {
		t.Fatal(pt)
	}
	// the user's version of a tool output: infuse's own message replaced, the rest kept
	out := "file content here\n" + m.String()
	d := displayFor("pt-BR", out, []Msg{m})
	if !strings.HasPrefix(d, "file content here\n[infuse] Bloqueado: é preciso um novo diagnóstico") {
		t.Fatal(d)
	}
	if displayFor("en", out, []Msg{m}) != "" {
		t.Fatal("an English reader gets no separate display")
	}
}
