package api

import (
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/llm"
	"infuse/internal/projectmap"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
)

// studioRoutes1 handles one group of studio routes.

func studioRoutes1(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root string, str func(string) string) bool {
	path := r.URL.Path
	switch {
	case path == "/api/settings" || path == "/api/settings/test":
		settingsAPI(w, r, path, body, str)
		return true
	case path == "/api/sessions" || strings.HasPrefix(path, "/api/sessions/"):
		legacySessions(w, r, body, root, str)
		return true
	case path == "/api/files/list":
		sendJSON(w, 200, projectFiles(root))
		return true
	case path == "/api/apps":
		sendJSON(w, 200, desktop.DetectApps())
		return true
	case path == "/api/apps/open":
		res, err := desktop.LaunchApp(str("id"), root)
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "launched": res})
		return true
	case path == "/api/providers/models":
		providerModels(w, r, body, str)
		return true
	}
	return false
}

func settingsAPI(w http.ResponseWriter, r *http.Request, path string, body map[string]any, str func(string) string) {
	if path == "/api/settings/test" {
		s := config.LoadSettings()
		if str("apiKey") != "" && !strings.Contains(str("apiKey"), "…") {
			s.APIKey = str("apiKey")
		}
		p, err := llm.NewProvider(s.Provider, llm.ProviderConfig{APIKey: s.APIKey, BaseURL: s.BaseURL})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		res, err := p.Stream(r.Context(), llm.ModelRequest{Model: s.Model, Messages: []core.Message{{Role: "user", Content: []core.Block{{Type: "text", Text: "Responda apenas: ok"}}}}, MaxTokens: 16}, func(llm.StreamEvent) {})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true, "model": s.Model, "usage": res.Usage})
		return
	}
	if r.Method == http.MethodPut {
		s := config.LoadSettings()
		key := s.APIKey
		b, _ := json.Marshal(body)
		if err := json.Unmarshal(b, &s); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		if s.APIKey == keyMask { // the mask coming back is not a new key
			s.APIKey = key
		}
		if err := config.SaveSettings(s); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true})
		return
	}
	s := config.LoadSettings()
	sendJSON(w, 200, map[string]any{"settings": maskSettings(s), "hasKey": s.APIKey != "", "providers": llm.ProviderNames(), "tools": agent.ToolSpecs()})
}

// keyMask stands for a stored key: no part of it leaves the server.
const keyMask = "…"

func maskSettings(s config.Settings) config.Settings {
	if s.APIKey != "" {
		s.APIKey = keyMask
	}
	return s
}

func projectFiles(root string) []string {
	// Flat list for the @ autocomplete: every file and directory the map knows, project-relative.
	m := projectmap.LoadOrBuildMap(root)
	out := []string{}
	seen := map[string]bool{}
	for p := range m.Files {
		out = append(out, p)
		for d := filepath.ToSlash(filepath.Dir(p)); d != "." && d != "/" && !seen[d]; d = filepath.ToSlash(filepath.Dir(d)) {
			seen[d] = true
			out = append(out, d+"/")
		}
	}
	sort.Strings(out)
	return out
}

func providerModels(w http.ResponseWriter, r *http.Request, body map[string]any, str func(string) string) {
	s := config.LoadSettings()
	name := str("provider")
	if name == "" {
		name = s.Provider
	}
	key := s.APIKey
	if k := str("apiKey"); k != "" && !strings.Contains(k, "…") {
		key = k
	}
	base := s.BaseURL
	if b, ok := body["baseUrl"].(string); ok {
		base = b
	}
	names, err := llm.ListModels(r.Context(), name, llm.ProviderConfig{APIKey: key, BaseURL: base})
	if err != nil {
		sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	sendJSON(w, 200, map[string]any{"ok": true, "models": names})
}
