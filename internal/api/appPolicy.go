package api

// appPolicy: the Rules page's routes — the project's base (choice, scaffold, custom editor), the
// compiled instructions and the project's prohibitions. These are the user's surfaces; the agent has
// no route here and touches the same files only through the gates.

import (
	"infuse/internal/agent"
	"infuse/internal/base"
	"infuse/internal/knowledge"
	"infuse/internal/projectmap"
	"net/http"
)

func appPolicy(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	switch {
	case path == "/api/base" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"catalog": base.Catalog(), "current": base.Load(root)})
		return true
	case path == "/api/base" && r.Method == http.MethodPut:
		err := saveBase(root, body)
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "current": base.Load(root)})
		return true
	case path == "/api/base/scaffold" && r.Method == http.MethodPost:
		b, err := base.Scaffold(root, str("base"))
		if err == nil {
			projectmap.WriteMap(root, projectmap.BuildMap(root)) // the map is pre-filled with the base's folders
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "base": b})
		return true
	case path == "/api/rules/compiled" && r.Method == http.MethodGet:
		sendJSON(w, 200, knowledge.LoadCompiledRules(root))
		return true
	case path == "/api/bans" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"sessions": agent.ProjectBans(root)})
		return true
	}
	return false
}

// saveBase records a starter by name, or a custom base from the editor's fields.
func saveBase(root string, body map[string]any) error {
	if name, ok := body["base"].(string); ok && name != "" {
		return base.Save(root, name, nil)
	}
	var custom base.Base
	custom.Name, _ = body["name"].(string)
	custom.DataDir, _ = body["dataDir"].(string)
	custom.Tokens, _ = body["tokens"].(string)
	if n, ok := body["maxFileLines"].(float64); ok {
		custom.MaxFileLines = int(n)
	}
	if list, ok := body["folders"].([]any); ok {
		for _, f := range list {
			if s, ok := f.(string); ok && s != "" {
				custom.Folders = append(custom.Folders, s)
			}
		}
	}
	return base.Save(root, "", &custom)
}
