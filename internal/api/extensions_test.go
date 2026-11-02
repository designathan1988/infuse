package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/config"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func extensionRequest(t *testing.T, method, target, body, root string) map[string]any {
	t.Helper()
	var m map[string]any
	if body != "" {
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
	}
	str := func(k string) string { s, _ := m[k].(string); return s }
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	q := map[string]string{}
	for k, v := range r.URL.Query() {
		q[k] = v[0]
	}
	w := httptest.NewRecorder()
	if !appExtensions(w, r, q, m, root, "", str, func(string) int { return 0 }, func(string) bool { return false }) {
		t.Fatal("not routed:", target)
	}
	var out map[string]any
	if json.Unmarshal(w.Body.Bytes(), &out) != nil {
		var list []any
		if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
			t.Fatal(w.Body.String())
		}
		return map[string]any{"list": list}
	}
	return out
}

func TestSettingsLayerRoutes(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if out := extensionRequest(t, "PUT", "/api/settings/local", `{"settings":{"env":{"K":"v"},"mode":"plan"}}`, root); out["ok"] != true {
		t.Fatalf("%+v", out)
	}
	s := config.EffectiveSettings(root)
	if s.Env["K"] != "v" || s.Mode != "plan" {
		t.Fatalf("%+v %s", s.Env, s.Mode)
	}
	out := extensionRequest(t, "GET", "/api/settings/layers", "", root)
	if len(out["list"].([]any)) != 4 {
		t.Fatalf("%+v", out)
	}
}

func TestLocalEnvValuesStayOnTheServer(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if out := extensionRequest(t, "PUT", "/api/settings/local", `{"settings":{"env":{"TOKEN":"s3cret","OLD":"gone"}}}`, root); out["ok"] != true {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "GET", "/api/settings/layers", "", root); strings.Contains(fmt.Sprint(out), "s3cret") {
		t.Fatalf("a value left the server: %+v", out)
	}
	// the mask coming back keeps the stored value; a key that was never stored cannot adopt it
	body := `{"settings":{"env":{"TOKEN":"` + keyMask + `","NEW":"value","GHOST":"` + keyMask + `"}}}`
	if out := extensionRequest(t, "PUT", "/api/settings/local", body, root); out["ok"] != true {
		t.Fatalf("%+v", out)
	}
	env := config.EffectiveSettings(root).Env
	if env["TOKEN"] != "s3cret" || env["NEW"] != "value" || len(env) != 2 {
		t.Fatalf("%+v", env)
	}
}

func TestLocalSettingsRefuseReservedVariables(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	out := extensionRequest(t, "PUT", "/api/settings/local", `{"settings":{"env":{"PATH":"x"}}}`, root)
	if out["ok"] == true || !strings.Contains(out["error"].(string), "PATH") {
		t.Fatalf("%+v", out)
	}
}

func TestMemoryNoteRoute(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if out := extensionRequest(t, "POST", "/api/memory/note", `{"text":"  use pnpm  "}`, root); out["ok"] != true {
		t.Fatalf("%+v", out)
	}
	items := agent.LoadMemory(agent.ProjectMemoryFile(root))
	if len(items) != 1 || items[0].Text != "use pnpm" || items[0].Origin != "usuario" {
		t.Fatalf("%+v", items)
	}
	if out := extensionRequest(t, "POST", "/api/memory/note", `{"text":"   "}`, root); out["ok"] == true {
		t.Fatal("an empty note")
	}
}

func TestStylesKindsAndPluginRoutes(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	if out := extensionRequest(t, "GET", "/api/output-styles", "", root); len(out["styles"].([]any)) < 4 {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "GET", "/api/agents/kinds", "", root); len(out["list"].([]any)) != 3 {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "GET", "/api/plugins", "", root); len(out["list"].([]any)) != 0 {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "POST", "/api/plugins/install", `{"url":"/etc"}`, root); out["ok"] == true {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "GET", "/api/plugins/marketplace", "", root); out["ok"] == true || !strings.Contains(out["error"].(string), "marketplaceUrl") {
		t.Fatalf("%+v", out)
	}
}

func TestTurnLogRouteFiltersOneSession(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(root, ".infuse", "turnlog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"v":1,"kind":"model","session":"s1","turn":1}` + "\n" + `{"v":1,"kind":"tool","session":"s1","turn":2,"tool":{"name":"Bash"}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := extensionRequest(t, "GET", "/api/turnlog?session=s1", "", root); len(out["list"].([]any)) != 2 {
		t.Fatalf("%+v", out)
	}
	if out := extensionRequest(t, "GET", "/api/turnlog?session=s1&turn=2&tool=bash", "", root); len(out["list"].([]any)) != 1 {
		t.Fatalf("%+v", out)
	}
}
