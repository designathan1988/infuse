package api

// appPolicy: the Rules page's routes — the project's base (choice, scaffold, custom editor), the
// behaviour policy, the compiled instructions and the project's prohibitions. These are the user's surfaces; the agent has
// no route here and touches the same files only through the gates.

import (
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/base"
	"infuse/internal/knowledge"
	"infuse/internal/policy"
	"net/http"
	"os"
)

func appPolicy(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	switch {
	case path == "/api/base" || path == "/api/base/scaffold":
		return baseRoute(w, r, body, root, str)
	case path == "/api/policy":
		policyRoute(w, r, body, root, q)
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

// policyRoute reads and writes the behaviour policy: one document per scope.
func policyRoute(w http.ResponseWriter, r *http.Request, body map[string]any, root string, q map[string]string) {
	global := q["scope"] == "global"
	if !global && root == "" {
		sendJSON(w, http.StatusBadRequest, map[string]any{"error": "project required"})
		return
	}
	switch r.Method {
	case http.MethodGet:
		p, err := policy.Effective(root)
		if err != nil {
			sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		file := policy.File(root, global)
		_, statErr := os.Stat(file)
		sendJSON(w, http.StatusOK, map[string]any{"policy": p, "file": file, "exists": statErr == nil, "global": global})
	case http.MethodPut:
		b, _ := json.Marshal(body)
		var p policy.Policy
		if err := json.Unmarshal(b, &p); err != nil {
			sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		if err := policy.Save(root, global, p); err != nil {
			sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"ok": true, "file": policy.File(root, global)})
	default:
		sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}
