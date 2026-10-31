package api

import (
	"encoding/json"
	"errors"
	"infuse/internal/mcp"
	"net/http"
	"strings"
)

// appMCP handles one group of application routes.

func appMCP(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- MCP connectors
	case path == "/api/mcp" && r.Method == http.MethodGet:
		sendJSON(w, 200, mcp.For(root).Status())
		return true
	case strings.HasPrefix(path, "/api/mcp/"):
		scope, name := str("scope"), str("name")
		if scope != "global" {
			scope = "project"
		}
		m := mcpFor(root)
		var err error
		switch strings.TrimPrefix(path, "/api/mcp/") {
		case "add": // a server from a preset or written by the user
			var cfg mcpServerConfig
			b, _ := json.Marshal(body["config"])
			if err = json.Unmarshal(b, &cfg); err == nil {
				if strings.TrimSpace(name) == "" || (cfg.Command == "" && cfg.URL == "") {
					err = errors.New("informe o nome e o comando (ou o endereço) do servidor")
				} else {
					err = editMCP(root, scope, func(sv map[string]mcpServerConfig) { sv[name] = cfg })
				}
			}
			if err == nil && boolean("approve") { // added by the user from the app: approved in the same click
				err = mcp.Approve(root, scope, name, cfg)
			}
		case "approve":
			cfg, ok := mcp.LoadFile(mcp.FilePath(root, scope)).Servers[name]
			if !ok {
				err = errors.New("servidor não encontrado")
			} else {
				err = mcp.Approve(root, scope, name, cfg)
			}
		case "toggle":
			err = editMCP(root, scope, func(sv map[string]mcpServerConfig) {
				if c, ok := sv[name]; ok {
					c.Disabled = boolean("disabled")
					sv[name] = c
				}
			})
		case "remove":
			err = editMCP(root, scope, func(sv map[string]mcpServerConfig) { delete(sv, name) })
		case "restart":
		default:
			sendJSON(w, 404, map[string]any{"error": "ação desconhecida"})
			return true
		}
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		m.Restart(r.Context(), scope, name)
		sendJSON(w, 200, map[string]any{"ok": true, "servers": m.Status()})
		return true
	}
	return false
}
