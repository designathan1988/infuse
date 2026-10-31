package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// appMemory handles one group of application routes.

func appMemory(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- structured memory (the user's curation)
	case path == "/api/memory/items" && r.Method == http.MethodGet:
		out := map[string]any{"project": loadMemory(projectMemoryFile(root)), "global": loadMemory(globalMemoryFile()), "session": []MemoryItem{}}
		if id := q["session"]; id != "" {
			if s, err := loadSession(root, id); err == nil {
				s.Mu.Lock()
				out["session"] = append([]MemoryItem{}, s.Memory...)
				s.Mu.Unlock()
			}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/memory/items/save":
		var items []MemoryItem
		b, _ := json.Marshal(body["items"])
		if err := json.Unmarshal(b, &items); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		clean := []MemoryItem{}
		for _, it := range items {
			if it.Origin == "" {
				it.Origin = "usuario"
			}
			if normalizeMemory(&it) == nil {
				clean = append(clean, it)
			}
		}
		var err error
		switch scope := str("scope"); scope {
		case "session":
			s, e := loadSession(root, str("session"))
			if e != nil {
				sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
				return true
			}
			err = s.EditMemory("session", func([]MemoryItem) []MemoryItem { return clean })
		case "global":
			err = saveMemory(globalMemoryFile(), clean)
		default:
			err = saveMemory(projectMemoryFile(root), clean)
		}
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "items": clean})
		return true
	case path == "/api/oracle/suggest":
		sendJSON(w, 200, SuggestChecks(root))
		return true
	case path == "/api/oracle/verdicts":
		sendJSON(w, 200, ListVerdicts(root, 20))
		return true
	case path == "/api/oracle/evidence":
		run, name := filepath.Base(q["run"]), filepath.Base(q["file"])
		b, err := os.ReadFile(filepath.Join(evidenceDir(root), run, name))
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "evidência não encontrada"})
			return true
		}
		sendJSON(w, 200, map[string]any{"name": name, "content": string(b)})
		return true
	case path == "/api/apps/command":
		// the shell command that starts a CLI app, for the terminal pane
		for _, a := range DetectApps() {
			if a.ID != str("id") || a.Kind != "cli" {
				continue
			}
			if !a.Found { // not installed: the terminal runs the official installer, visibly
				if pkg := cliPackages[a.ID]; pkg != "" && str("install") != "" {
					sendJSON(w, 200, map[string]any{"ok": true, "command": "npm install -g " + pkg, "name": "instalar " + a.Name, "installing": true})
					return true
				}
				break
			}
			cmd := shellQuote(toBashPath(a.Path))
			if a.ID == "codex-cli" && strings.Contains(a.Path, "WindowsApps") {
				cmd += " --no-daemon" // the CLI bundled with the Codex app needs this outside the app's background server
			}
			sendJSON(w, 200, map[string]any{"ok": true, "command": cmd, "name": a.Name})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": false, "error": "aplicativo não encontrado"})
		return true
	case path == "/api/models":
		s := LoadSettings()
		changed := false
		for id, c := range s.Connections {
			if c.Connected && len(c.Models) == 0 && c.Kind != "mock" {
				kind, cfg := connectionConfig(s, id)
				if names, err := ListModels(r.Context(), kind, cfg); err == nil && len(names) > 0 {
					c.Models = names
					s.Connections[id] = c
					changed = true
				}
			}
		}
		if changed {
			_ = SaveSettings(s)
		}
		sendJSON(w, 200, AvailableModels(s))
		return true
	case path == "/api/usage":
		sendJSON(w, 200, UsageTotals())
		return true
	}
	return false
}
