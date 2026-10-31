package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConventionsAreCountedFromTheCode(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	for i, name := range []string{"user-list.js", "order-form.js", "api-client.js", "login.test.js"} {
		code := "const x = require('./a');\nfunction f() {\n  // retorna o usuário que fez a compra\n  return 1;\n}\n"
		if i == 3 {
			code = "const { test } = require('node:test');\ntest('x', () => {\n  return 1;\n});\n"
		}
		_ = os.WriteFile(filepath.Join(root, "src", name), []byte(code), 0o644)
	}
	text := LearnConventions(root)
	for _, want := range []string{"Indentation: 2 spaces", "Quotes in imports: single quotes", "Modules: CommonJS (require)", "End of statement (JS/TS): with semicolons", "File names .js: kebab-case", "Tests: *.test.js", "Comment language: Portuguese"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	s := newSession(root)
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), "kebab-case") {
		t.Fatal("conventions not given to the agent")
	}
	if _, err := os.Stat(conventionsFile(root)); err != nil {
		t.Fatal("conventions not kept for the user to edit")
	}
}

func TestPinnedDocsFetchedAndListed(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>API do Widget</title><script>var x=1</script></head><body><nav>menu</nav><main><h1>widget.render(el)</h1><p>Desenha o widget dentro de el.</p></main></body></html>`)
	}))
	defer srv.Close()
	defer closeAgentBrowsers()
	doc, err := PinDoc(context.Background(), root, srv.URL+"/api")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc.File)))
	if !strings.Contains(string(b), "Desenha o widget dentro de el.") || strings.Contains(string(b), "var x=1") {
		t.Fatalf("%s", b)
	}
	s := newSession(root)
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), doc.File) {
		t.Fatal("pinned doc not listed for the agent")
	}
	_ = os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"dependencies":{"react":"19","left-pad":"1"}}`), 0o644)
	sug := SuggestDocs(root)
	if len(sug) != 1 || sug[0].Name != "React" {
		t.Fatalf("%+v", sug)
	}
	if err := unpinDoc(root, doc.URL); err != nil || len(loadDocs(root)) != 0 {
		t.Fatal("unpin")
	}
}

func TestDiagnosticsAfterWrite(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	bad := filepath.Join(root, "x.json")
	_ = os.WriteFile(bad, []byte(`{"a": }`), 0o644)
	if d := fileDiagnostics(root, bad); !strings.Contains(d, "JSON inválido") {
		t.Fatal(d)
	}
	js := filepath.Join(root, "a.js")
	_ = os.WriteFile(js, []byte("function ( {\n"), 0o644)
	if _, err := os.Stat(js); err == nil {
		if d := fileDiagnostics(root, js); d == "" && hasNode() {
			t.Fatal("broken JS not reported")
		}
	}
	_ = os.WriteFile(js, []byte("module.exports = 1;\n"), 0o644)
	if d := fileDiagnostics(root, js); d != "" {
		t.Fatal(d)
	}
}

func hasNode() bool { _, err := exec.LookPath("node"); return err == nil }
