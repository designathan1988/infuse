package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infuse/internal/base"
	"infuse/internal/projectmap"
)

// The prompt states the base's intended structure against what the project actually has.
func TestBaseGroundsShowsIntendedVersusActual(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := base.Save(root, "web", nil); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(root, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "src", "a.js"), []byte("export const a = 1;\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "scripts", "deploy.js"), []byte("export const d = 1;\n"), 0o644)
	m := projectmap.BuildMap(root)
	projectmap.WriteMap(root, m)
	g := baseGrounds(root)
	if !strings.Contains(g, "declared folders not present yet") || !strings.Contains(g, "ui") {
		t.Fatalf("missing intended folders not stated: %q", g)
	}
	if !strings.Contains(g, "code folders outside the base") || !strings.Contains(g, "scripts") {
		t.Fatalf("out-of-base code not stated: %q", g)
	}
}
