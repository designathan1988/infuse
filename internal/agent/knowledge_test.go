package agent

import (
	"context"
	"infuse/internal/journal"
	"infuse/internal/knowledge"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// An observed vice reaches the prompt with its deterministic signature and its remedy, not only a
// name and a count.
func TestLessonsCarryTheirRemedy(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	journal.Lesson(root, "swallowed-error", "src/a.js: empty catch")
	s := newSession(root)
	p := buildSystemPrompt(s, &RuleSet{})
	for _, want := range []string{"swallowed-error", "signature:", "remedy:", "propagate or handle it"} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in the prompt", want)
		}
	}
}

// The generated feature index reaches the agent prompt.
func TestPromptCarriesTheFeatureIndex(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	dir := filepath.Join(root, "src", "features", "billing")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "api.ts"), []byte("export const load = 1\n"), 0o644)
	p := buildSystemPrompt(newSession(root), &RuleSet{})
	if !strings.Contains(p, "Feature index") || !strings.Contains(p, "billing") {
		t.Fatalf("feature index missing from the prompt:\n%s", p)
	}
}

// A folder's instruction file is shown once, when work enters that folder.
func TestFolderInstructionsLoadOnce(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src", "api"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "CLAUDE.md"), []byte("folder rule: no globals"), 0o644)
	s := newSession(root)
	got := s.localInstructions(filepath.Join(root, "src", "api"))
	if !strings.Contains(got, "no globals") || !strings.Contains(got, "src/CLAUDE.md") {
		t.Fatalf("folder instructions missing: %q", got)
	}
	if again := s.localInstructions(filepath.Join(root, "src", "api")); again != "" {
		t.Fatalf("folder instructions repeated: %q", again)
	}
	if rootOnly := s.localInstructions(root); rootOnly != "" {
		t.Fatalf("root instructions are already in the prompt: %q", rootOnly)
	}
}

// Rules compiled from an older instruction text stay enforced but are labeled stale in the prompt.
func TestStaleCompiledRulesAreLabeledAndEnforced(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("Read docs/guide.md before changing anything.\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "docs"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("# Guide\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".infuse"), 0o755)
	stale := `{"hash":"old","compiled":1,"model":"mock","stale":true,"rules":[{"kind":"read","path":"docs/guide.md","quote":"Read docs/guide.md before changing anything."}]}`
	if err := os.WriteFile(filepath.Join(root, ".infuse", "project-rules.json"), []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	knowledge.InvalidateRules(root)
	p := instructionsPrompt(root)
	if !strings.Contains(p, "compiled from an earlier version") || !strings.Contains(p, "docs/guide.md") {
		t.Fatalf("stale rules not labeled/enforced: %q", p)
	}
	if !knowledge.LoadProjectRules(root).Stale {
		t.Fatal("the stale flag did not reach the rules")
	}
}

// GEMINI.md, .cursorrules and copilot instructions are instruction layers like CLAUDE.md/AGENTS.md.
func TestForeignInstructionFilesAreInjected(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("gemini says: use tabs"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".cursorrules"), []byte("cursor says: no globals"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, ".github"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".github", "copilot-instructions.md"), []byte("copilot says: test first"), 0o644)
	p := buildSystemPrompt(newSession(root), &RuleSet{})
	for _, want := range []string{"use tabs", "no globals", "test first"} {
		if !strings.Contains(p, want) {
			t.Fatalf("instruction layer missing: %q", want)
		}
	}
}

func TestConventionsAreCountedFromTheCode(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	for i, name := range []string{"user-list.js", "order-form.js", "api-client.js", "login.test.js"} {
		code := "const x = require('./a');\nfunction f() {\n  // returns the user who made the purchase\n  return 1;\n}\n"
		if i == 3 {
			code = "const { test } = require('node:test');\ntest('x', () => {\n  return 1;\n});\n"
		}
		_ = os.WriteFile(filepath.Join(root, "src", name), []byte(code), 0o644)
	}
	text := LearnConventions(root)
	for _, want := range []string{"Indentation: 2 spaces", "Quotes in imports: single quotes", "Modules: CommonJS (require)", "End of statement (JS/TS): with semicolons", "File names .js: kebab-case", "Tests: *.test.js", "Observed, not a rule", "Comments are mostly in English"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in\n%s", want, text)
		}
	}
	if strings.Contains(text, "- Comment language:") { // a fact about old code is not a convention to follow
		t.Fatal("comment language is still taught as a convention")
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
		_, _ = io.WriteString(w, `<html><head><title>Widget API</title><script>var x=1</script></head><body><nav>menu</nav><main><h1>widget.render(el)</h1><p>Draws the widget inside el.</p></main></body></html>`)
	}))
	defer srv.Close()
	defer closeAgentBrowsers()
	doc, err := PinDoc(context.Background(), root, srv.URL+"/api")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(doc.File)))
	if !strings.Contains(string(b), "Draws the widget inside el.") || strings.Contains(string(b), "var x=1") {
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
	if d := fileDiagnostics(root, bad); !strings.Contains(d, "invalid JSON") {
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
