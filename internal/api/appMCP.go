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
		mcpAction(w, r, path, body, root, str, boolean)
		return true
	}
	return false
}

func mcpAction(w http.ResponseWriter, r *http.Request, path string, body map[string]any, root string, str func(string) string, boolean func(string) bool) {
	scope, name := str("scope"), str("name")
	if scope != "global" {
		scope = "project"
	}
	m := mcp.For(root)
	var err error
	switch strings.TrimPrefix(path, "/api/mcp/") {
	case "add": // a server from a preset or written by the user
		err = mcpAddServer(body, root, scope, name, boolean("approve"))
	case "approve":
		err = mcpApproveServer(root, scope, name)
	case "toggle":
		err = mcpToggleServer(root, scope, name, boolean("disabled"))
	case "remove":
		err = mcpRemoveServer(root, scope, name)
	case "restart":
	default:
		sendJSON(w, 404, map[string]any{"error": "ação desconhecida"})
		return
	}
	if err != nil {
		sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	m.Restart(r.Context(), scope, name)
	sendJSON(w, 200, map[string]any{"ok": true, "servers": m.Status()})
}

func mcpAddServer(body map[string]any, root, scope, name string, approve bool) error {
	var cfg mcp.ServerConfig
	b, _ := json.Marshal(body["config"])
	err := json.Unmarshal(b, &cfg)
	if err == nil {
		if strings.TrimSpace(name) == "" || (cfg.Command == "" && cfg.URL == "") {
			err = errors.New("informe o nome e o comando (ou o endereço) do servidor")
		} else {
			err = mcp.Edit(root, scope, func(sv map[string]mcp.ServerConfig) {
				sv[name] = cfg
			})
		}
	}
	if err == nil && approve { // added by the user from the app: approved in the same click
		err = mcp.Approve(root, scope, name, cfg)
	}
	return err
}

func mcpApproveServer(root, scope, name string) error {
	cfg, ok := mcp.LoadFile(mcp.FilePath(root, scope)).Servers[name]
	if !ok {
		return errors.New("servidor não encontrado")
	}
	return mcp.Approve(root, scope, name, cfg)
}

func mcpToggleServer(root, scope, name string, disabled bool) error {
	return mcp.Edit(root, scope, func(sv map[string]mcp.ServerConfig) {
		if c, ok := sv[name]; ok {
			c.Disabled = disabled
			sv[name] = c
		}
	})
}

func mcpRemoveServer(root, scope, name string) error {
	return mcp.Edit(root, scope, func(sv map[string]mcp.ServerConfig) {
		delete(sv, name)
	})
}
