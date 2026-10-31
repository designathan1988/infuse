package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/llm"
	"infuse/internal/projectmap"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// studioRoutes1 handles one group of studio routes.

func studioRoutes1(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root string, str func(string) string) bool {
	path := r.URL.Path
	switch {
	case path == "/api/settings":
		if r.Method == http.MethodPut {
			s := config.LoadSettings()
			b, _ := json.Marshal(body)
			if err := json.Unmarshal(b, &s); err != nil {
				sendJSON(w, 400, map[string]any{"error": err.Error()})
				return true
			}
			if err := config.SaveSettings(s); err != nil {
				sendJSON(w, 500, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true})
			return true
		}
		s := config.LoadSettings()
		masked := s
		if len(masked.APIKey) > 8 {
			masked.APIKey = masked.APIKey[:6] + "…" + masked.APIKey[len(masked.APIKey)-4:]
		}
		sendJSON(w, 200, map[string]any{"settings": masked, "hasKey": s.APIKey != "", "providers": llm.ProviderNames(), "tools": agent.ToolSpecs()})
		return true

	case path == "/api/settings/test":
		s := config.LoadSettings()
		if str("apiKey") != "" && !strings.Contains(str("apiKey"), "…") {
			s.APIKey = str("apiKey")
		}
		p, err := llm.NewProvider(s.Provider, llm.ProviderConfig{APIKey: s.APIKey, BaseURL: s.BaseURL})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		res, err := p.Stream(r.Context(), llm.ModelRequest{Model: s.Model, Messages: []core.Message{{Role: "user", Content: []core.Block{{Type: "text", Text: "Responda apenas: ok"}}}}, MaxTokens: 16}, func(llm.StreamEvent,

		) {
		})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "model": s.Model, "usage": res.Usage})
		return true

	case path == "/api/sessions" && r.Method == http.MethodGet:
		sendJSON(w, 200, agent.ListSessions(root))
		return true
	case path == "/api/sessions" && r.Method == http.MethodPost:
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "projeto não encontrado"})
			return true
		}
		s := agent.NewSession(root)
		sendJSON(w, 200, s.Snapshot())
		return true

	case strings.HasPrefix(path, "/api/sessions/"):
		rest := strings.TrimPrefix(path, "/api/sessions/")
		parts := strings.SplitN(rest, "/", 2)
		id := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}
		s, err := agent.LoadSession(root, id)
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
			return true
		}
		switch action {
		case "":
			switch r.Method {
			case http.MethodDelete:
				s.Stop()
				agent.RemoveSession(root, id)
				sendJSON(w, 200, map[string]any{"ok": true})
			case http.MethodPut:
				s.Mu.Lock()
				if t := str("title"); t != "" {
					s.Title = t
				}
				if m := str("model"); m != "" {
					s.Model = m
				}
				s.Mu.Unlock()
				s.Save()
				sendJSON(w, 200, s.Snapshot())
			default:
				sendJSON(w, 200, s.Snapshot())
			}
		case "duplicate":
			d := agent.NewSession(root)
			d.Mu.Lock()
			d.Title = s.Title + " (cópia)"
			d.Model = s.Model
			d.Messages = s.CopyMessages()
			d.Turn = s.Turn
			d.TurnStarts = append([]int{}, s.TurnStarts...)
			d.Mu.Unlock()
			d.Save()
			sendJSON(w, 200, d.Snapshot())
		case "send":
			var atts []core.Attachment

			ab, _ := json.Marshal(body["attachments"])
			_ = json.Unmarshal(ab, &atts)
			var imgs []core.InlineImage

			ib, _ := json.Marshal(body["images"])
			_ = json.Unmarshal(ib, &imgs)
			if err := s.Send(str("text"), atts, imgs); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "turn": s.Turn})
		case "stop":
			s.Stop()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "permission":
			sendJSON(w, 200, map[string]any{"ok": s.ResolvePermission(str("id"), str("decision"))})
		case "answer":
			sendJSON(w, 200, map[string]any{"ok": s.Answer(str("id"), str("answer"))})
		case "revert":
			t, _ := strconv.Atoi(fmt.Sprint(body["turn"]))
			if err := s.Revert(t); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, s.Snapshot())
		case "events":
			streamEvents(w, r, s)
		default:
			http.NotFound(w, r)
		}
		return true

	case path == "/api/files/list":
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
		sendJSON(w, 200, out)
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
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "models": names})
		return true
	}
	return false
}
