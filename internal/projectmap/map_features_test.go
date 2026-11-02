package projectmap

import (
	"encoding/json"
	"strings"
	"testing"
)

func hasString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func featureByName(m *ProjectMap, name string) *Feature {
	for i := range m.Features {
		if strings.EqualFold(m.Features[i].Name, name) {
			return &m.Features[i]
		}
	}
	return nil
}

func hasRoute(routes []Route, method, path string) bool {
	for _, r := range routes {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

// The feature index reads feature folders, per-command files, route literals and filename prefixes.
func TestFeatureIndex(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/features/billing/ui.ts", "export const Card = 1\n")
	writeMapFile(t, root, "src/features/billing/api.ts", "export const load = 1\n")
	writeMapFile(t, root, "src/pages/checkout/page.tsx", "export default function P() { return null }\n")
	writeMapFile(t, root, "internal/commands/deploy.go", "package commands\n\nfunc Run() {}\n")
	writeMapFile(t, root, "internal/api/users.go", "package api\n\nfunc routes() { mux.HandleFunc(\"GET /users\", list) }\n")
	writeMapFile(t, root, "reports_store.py", "def save():\n    pass\n")
	writeMapFile(t, root, "reports_view.py", "def view():\n    pass\n")

	m := BuildMap(root)
	if f := featureByName(m, "billing"); f == nil || f.FileCount != 2 || !hasString(f.Paths, "src/features/billing") {
		t.Fatalf("folder feature: %+v", f)
	}
	if f := featureByName(m, "checkout"); f == nil || !hasString(f.Paths, "src/pages/checkout") {
		t.Fatalf("page feature: %+v", f)
	}
	if f := featureByName(m, "deploy"); f == nil || !hasString(f.Paths, "internal/commands/deploy.go") {
		t.Fatalf("command file feature: %+v", f)
	}
	if f := featureByName(m, "reports"); f == nil || f.Source != "prefix" || f.FileCount != 2 {
		t.Fatalf("prefix fallback: %+v", f)
	}
	if f := featureByName(m, "users"); f == nil || f.Source != "route" || !hasString(f.Routes, "GET /users") {
		t.Fatalf("route feature: %+v", f)
	}
	if !hasRoute(m.Routes, "GET", "/users") {
		t.Fatalf("route not listed: %+v", m.Routes)
	}
	summary := m.FeatureSummary(12)
	if !strings.Contains(summary, "billing") || !strings.Contains(summary, "GET /users") {
		t.Fatalf("summary: %s", summary)
	}
}

// Route literals from the supported frameworks are read with their method and path.
func TestRoutesFromSourceLiterals(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "api/app.py", "@app.get(\"/health\")\n@app.route(\"/items\", methods=[\"GET\", \"POST\"])\n")
	writeMapFile(t, root, "web/routes.ts", "router.post('/orders', h)\napp.get('/orders/:id', h)\n")
	writeMapFile(t, root, "web/client.ts", "api.get('/client-only', h)\n")
	writeMapFile(t, root, "Controllers/UserController.cs", "[HttpGet(\"users\")]\n[Route(\"admin\")]\n")
	writeMapFile(t, root, "server/src/main.rs", "#[get(\"/rusty\")]\n")
	writeMapFile(t, root, "server/src/axum.rs", "let app = Router::new().route(\"/axum\", get(hi));\n")
	writeMapFile(t, root, "routes.php", "Route::delete('/session', h);\n")
	writeMapFile(t, root, "config/routes.rb", "get '/home', to: 'pages#home'\n")
	m := BuildMap(root)
	for _, want := range [][2]string{
		{"GET", "/health"}, {"GET", "/items"}, {"POST", "/items"},
		{"POST", "/orders"}, {"GET", "/orders/:id"},
		{"GET", "/users"}, {"*", "/admin"}, {"GET", "/rusty"}, {"*", "/axum"},
		{"DELETE", "/session"}, {"GET", "/home"},
	} {
		if !hasRoute(m.Routes, want[0], want[1]) {
			t.Errorf("missing route %s %s in %+v", want[0], want[1], m.Routes)
		}
	}
	// An HTTP client instance named api is not a route declaration.
	if hasRoute(m.Routes, "GET", "/client-only") {
		t.Errorf("a client call was read as a route: %+v", m.Routes)
	}
}

// Two builds of the same tree produce the same feature index and route list.
func TestFeatureIndexIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeMapFile(t, root, "src/features/a/x.ts", "export const X = 1\n")
	writeMapFile(t, root, "src/features/b/y.ts", "export const Y = 1\n")
	writeMapFile(t, root, "orders_a.py", "x = 1\n")
	writeMapFile(t, root, "orders_b.py", "y = 1\n")
	writeMapFile(t, root, "web/app.js", "app.get('/a', h)\napp.get('/b', h)\n")
	first, _ := json.Marshal(BuildMap(root))
	second, _ := json.Marshal(BuildMap(root))
	var a, b ProjectMap
	_ = json.Unmarshal(first, &a)
	_ = json.Unmarshal(second, &b)
	aJSON, _ := json.Marshal(struct {
		F []Feature
		R []Route
	}{a.Features, a.Routes})
	bJSON, _ := json.Marshal(struct {
		F []Feature
		R []Route
	}{b.Features, b.Routes})
	if string(aJSON) != string(bJSON) {
		t.Fatalf("feature index is not deterministic:\n%s\n%s", aJSON, bJSON)
	}
}
