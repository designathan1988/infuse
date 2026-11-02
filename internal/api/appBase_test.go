package api

import (
	"encoding/json"
	"infuse/internal/base"
	"infuse/internal/projectmap"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func callBase(t *testing.T, method, path, root string, body map[string]any) (int, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, strings.NewReader(string(b)))
	w := httptest.NewRecorder()
	if !baseRoute(w, r, body, root, func(k string) string { s, _ := body[k].(string); return s }) {
		t.Fatalf("route %s not handled", path)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestBaseAPIListsAppliesAndScaffolds(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	code, view := callBase(t, http.MethodGet, "/api/base", root, nil)
	if code != 200 || len(view["catalog"].([]any)) < 4 {
		t.Fatalf("catalog: %d %+v", code, view)
	}
	code, view = callBase(t, http.MethodPut, "/api/base", root, map[string]any{"base": "web"})
	if code != 200 || view["name"] != "web" || !view["exists"].(bool) {
		t.Fatalf("apply: %d %+v", code, view)
	}
	if b := base.Load(root); b == nil || b.Name != "web" {
		t.Fatalf("base file: %+v", b)
	}
	if _, err := os.Stat(projectmap.MapFile(root)); err != nil {
		t.Fatalf("map was not generated: %v", err)
	}

	parent := t.TempDir()
	code, out := callBase(t, http.MethodPost, "/api/base/scaffold", "", map[string]any{"parent": parent, "name": "new-app", "base": "cli"})
	if code != 200 || out["root"] != filepath.Join(parent, "new-app") {
		t.Fatalf("scaffold: %d %+v", code, out)
	}
	target := filepath.Join(parent, "new-app")
	if b := base.Load(target); b == nil || b.Name != "cli" {
		t.Fatalf("scaffolded base: %+v", b)
	}
	if _, err := os.Stat(projectmap.MapFile(target)); err != nil {
		t.Fatalf("scaffolded map: %v", err)
	}
	for _, unsafe := range []string{"../escape", "..", ".", "a/b"} {
		if code, _ := callBase(t, http.MethodPost, "/api/base/scaffold", "", map[string]any{"parent": parent, "name": unsafe, "base": "cli"}); code != 400 {
			t.Fatalf("unsafe name %q accepted: %d", unsafe, code)
		}
	}
}
