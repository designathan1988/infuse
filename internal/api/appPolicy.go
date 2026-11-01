package api

// The behaviour policy: one document per scope, read and written by the Rules surface.

import (
	"encoding/json"
	"infuse/internal/policy"
	"net/http"
	"os"
)

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
