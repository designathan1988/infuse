package api

import (
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/config"
	"infuse/internal/connections"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/llm"
	"infuse/internal/oracle"
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
		out := map[string]any{"project": agent.LoadMemory(agent.ProjectMemoryFile(root)), "global": agent.LoadMemory(agent.GlobalMemoryFile()), "session": []agent.MemoryItem{}}
		if id := q["session"]; id != "" {
			if s, err := agent.LoadSession(root, id); err == nil {
				s.Mu.Lock()
				out["session"] = append([]agent.MemoryItem{}, s.Memory...)
				s.Mu.Unlock()
			}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/memory/items/save":
		var items []agent.MemoryItem

		b, _ := json.Marshal(body["items"])
		if err := json.Unmarshal(b, &items); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		clean := []agent.MemoryItem{}
		for _, it := range items {
			if it.Origin == "" {
				it.Origin = "usuario"
			}
			if agent.NormalizeMemory(&it) == nil {
				clean = append(clean, it)
			}
		}
		var err error
		switch scope := str("scope"); scope {
		case "session":
			s, e := agent.LoadSession(root, str("session"))
			if e != nil {
				sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
				return true
			}
			err = s.EditMemory("session", func([]agent.MemoryItem,

			) []agent.MemoryItem {
				return clean
			})
		case "global":
			err = agent.SaveMemory(agent.GlobalMemoryFile(), clean)
		default:
			err = agent.SaveMemory(agent.ProjectMemoryFile(root), clean)
		}
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "items": clean})
		return true
	case path == "/api/oracle/suggest":
		sendJSON(w, 200, oracle.SuggestChecks(root))
		return true
	case path == "/api/oracle/verdicts":
		sendJSON(w, 200, oracle.ListVerdicts(root, 20))
		return true
	case path == "/api/oracle/evidence":
		run, name := filepath.Base(q["run"]), filepath.Base(q["file"])
		b, err := os.ReadFile(filepath.Join(oracle.EvidenceDir(root), run, name))
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "evidência não encontrada"})
			return true
		}
		sendJSON(w, 200, map[string]any{"name": name, "content": string(b)})
		return true
	case path == "/api/apps/command":
		// the shell command that starts a CLI app, for the terminal pane
		for _, a := range desktop.DetectApps() {
			if a.ID != str("id") || a.Kind != "cli" {
				continue
			}
			if !a.Found { // not installed: the terminal runs the official installer, visibly
				if pkg := desktop.CliPackages[a.ID]; pkg != "" && str("install") != "" {
					sendJSON(w, 200, map[string]any{"ok": true, "command": "npm install -g " + pkg, "name": "instalar " + a.Name, "installing": true})
					return true
				}
				break
			}
			cmd := filter.ShellQuote(core.ToBashPath(a.Path))
			if a.ID == "codex-cli" && strings.Contains(a.Path, "WindowsApps") {
				cmd += " --no-daemon" // the CLI bundled with the Codex app needs this outside the app's background server
			}
			sendJSON(w, 200, map[string]any{"ok": true, "command": cmd, "name": a.Name})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": false, "error": "aplicativo não encontrado"})
		return true
	case path == "/api/models":
		s := config.LoadSettings()
		changed := false
		for id, c := range s.Connections {
			if c.Connected && len(c.Models) == 0 && c.Kind != "mock" {
				kind, cfg := connections.
					ConnectionConfig(s, id)
				if names, err := llm.ListModels(r.Context(), kind, cfg); err == nil && len(names) > 0 {
					c.Models = names
					s.Connections[id] = c
					changed = true
				}
			}
		}
		if changed {
			_ = config.SaveSettings(s)
		}
		sendJSON(w, 200, connections.
			AvailableModels(s))
		return true
	case path == "/api/usage":
		sendJSON(w, 200, config.UsageTotals())
		return true
	}
	return false
}
