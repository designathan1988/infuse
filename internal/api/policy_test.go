package api

import (
	"encoding/json"
	"infuse/internal/base"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func policyRequest(t *testing.T, method, target, body string, root string) *httptest.ResponseRecorder {
	t.Helper()
	var m map[string]any
	if body != "" {
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
	}
	str := func(k string) string { s, _ := m[k].(string); return s }
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	appPolicy(w, r, map[string]string{"root": root}, m, root, "", str, func(string) int { return 0 }, func(string) bool { return false })
	return w
}

func TestBaseRoutesServeCatalogChoiceAndScaffold(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	var out struct {
		Catalog []base.Base `json:"catalog"`
		Current *base.Base  `json:"current"`
	}
	w := policyRequest(t, "GET", "/api/base", "", root)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/base: %d", w.Code)
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Catalog) < 4 || len(out.Catalog) != len(base.Catalog()) || out.Current != nil {
		t.Fatalf("catalog of %d, current %+v", len(out.Catalog), out.Current)
	}
	w = policyRequest(t, "PUT", "/api/base", `{"base":"web"}`, root)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT /api/base: %d", w.Code)
	}
	if b := base.Load(root); b == nil || b.Name != "web" {
		t.Fatalf("choice not recorded: %+v", b)
	}
	// an unknown starter is refused with the catalog named
	w = policyRequest(t, "PUT", "/api/base", `{"base":"nope"}`, root)
	if !strings.Contains(w.Body.String(), "nope") {
		t.Fatalf("unknown base accepted: %s", w.Body.String())
	}
	// a custom base needs a folder
	w = policyRequest(t, "PUT", "/api/base", `{"name":"x","folders":[]}`, root)
	if !strings.Contains(w.Body.String(), "folder") {
		t.Fatalf("folderless custom base accepted: %s", w.Body.String())
	}
}

func TestScaffoldRoutePrefillsTheMapAndBansRouteReads(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	w := policyRequest(t, "POST", "/api/base/scaffold", `{"base":"cli"}`, root)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("scaffold: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "internal", "commands")); err != nil {
		t.Fatal("the scaffold did not create the folders")
	}
	if _, err := os.Stat(filepath.Join(root, ".infuse", "map", "intended.json")); err != nil {
		t.Fatal("the intended map was not recorded")
	}
	// the bans route answers, empty when nothing was forbidden: a list, also when sessions exist
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	w = policyRequest(t, "GET", "/api/bans", "", root)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"sessions":[]`) {
		t.Fatalf("bans: %d %s", w.Code, w.Body.String())
	}
}
