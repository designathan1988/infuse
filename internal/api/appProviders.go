package api

import (
	"infuse/internal/config"
	"infuse/internal/connections"
	"net/http"
)

// appProviders handles one group of application routes.

func appProviders(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- providers and models
	case path == "/api/connections" && r.Method == http.MethodGet:
		s := config.LoadSettings()
		out := []map[string]any{}
		for _, p := range config.ProviderPresets {
			c := s.Connections[p.ID]
			base := c.BaseURL
			if base == "" {
				base = p.BaseURL
			}
			models := c.Models
			if models == nil {
				models = []string{}
			}
			out = append(out, map[string]any{"id": p.ID, "kind": p.Kind, "label": p.Label, "baseUrl": base, "defaultBaseUrl": p.BaseURL,
				"needKey": p.NeedKey, "keyUrl": p.KeyURL, "local": p.Local, "connected": c.Connected, "keyHint": c.KeyHint,
				"models": models, "default": s.Provider == p.ID})
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/connections/connect":
		c, err := connections.
			ConnectProvider(r.Context(), str("id"), str("apiKey"), str("baseUrl"))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "connection": c})
		return true
	case path == "/api/connections/disconnect":
		_ = connections.
			DisconnectProvider(str("id"))
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	case path == "/api/connections/default":
		s := config.LoadSettings()
		if id := str("id"); s.Connections[id].Connected {
			s.Provider = id
			if m := str("model"); m != "" {
				s.Model = m
			}
			_ = config.SaveSettings(s)
		}
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	}
	return false
}
