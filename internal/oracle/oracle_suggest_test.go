package oracle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestChecksFromProject(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"scripts":{"test":"vitest run","build":"vite build"},"devDependencies":{"vite":"5","typescript":"5"}}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "package-lock.json"), []byte(`{}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "tsconfig.json"), []byte(`{}`), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "a.ts"), []byte("export {}\n"), 0o644)
	got := map[string]CheckSuggestion{}
	for _, s := range SuggestChecks(root) {
		got[s.Check.ID] = s
	}
	if got["testes"].Check.Run != "npm test" || got["testes"].Setup != "npm ci" {
		t.Fatalf("%+v", got["testes"])
	}
	if got["build"].Check.Run != "npm run build" || got["tipos"].Check.Run != "npx tsc --noEmit" {
		t.Fatalf("%+v", got)
	}
	if got["pagina"].Check.URL != "http://localhost:5173/" || got["tamanho"].Check.Glob != "**/*.ts" {
		t.Fatalf("%+v", got)
	}
	// a suggestion already in the spec is not offered again
	writeSpec(t, root, `{"checks":[{"id":"x","kind":"command","run":"npm test"}]}`)
	for _, s := range SuggestChecks(root) {
		if s.Check.Run == "npm test" {
			t.Fatal("offered twice")
		}
	}
}
