package api

import (
	"context"
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
	if strings.TrimPrefix(path, "/api/mcp/") == "oauth" { // the browser sign-in the user asked for
		mcpOAuth(w, r, root, scope, name)
		return
	}
	if scope != "global" && !strings.HasPrefix(scope, mcp.PluginScope) {
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
		sendJSON(w, 404, map[string]any{"error": "unknown action", "code": "mcp.unknownAction"})
		return
	}
	if err != nil {
		sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	m.Restart(r.Context(), scope, name)
	sendJSON(w, 200, map[string]any{"ok": true, "servers": m.Status()})
}

// mcpOAuth starts the sign-in and answers with the address to open; when it finishes the server restarts
// with its new token.
func mcpOAuth(w http.ResponseWriter, r *http.Request, root, scope, name string) {
	m := mcp.For(root)
	for _, e := range m.Entries() {
		if e.Name != name || (scope != "" && e.Scope != scope) {
			continue
		}
		restart := func(err error) {
			if err == nil {
				m.Restart(context.Background(), e.Scope, e.Name)
			}
		}
		u, err := mcp.BeginOAuth(r.Context(), root, e.Scope, e.Name, e.Cfg, restart)
		sendJSON(w, 200, map[string]any{"ok": err == nil, "url": u, "error": errString(err)})
		return
	}
	sendJSON(w, 200, map[string]any{"ok": false, "error": "server not found"})
}

func mcpAddServer(body map[string]any, root, scope, name string, approve bool) error {
	var cfg mcp.ServerConfig
	b, _ := json.Marshal(body["config"])
	err := json.Unmarshal(b, &cfg)
	if err == nil {
		if strings.TrimSpace(name) == "" || (cfg.Command == "" && cfg.URL == "") {
			err = errors.New("enter the server's name and command (or address)")
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
		return errors.New("server not found")
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
