package mcp

// MCP connectors (Model Context Protocol). Servers are declared like in Claude Code: .mcp.json in the
// project and ~/.infuse/mcp.json for every project, {"mcpServers": {name: {command, args, env} or
// {type: "http", url, headers}}}. A server runs only after the user approves that exact configuration
// (the agent can write .mcp.json; approvals live in ~/.infuse). Its tools reach the agent as
// mcp__<server>__<tool>; calls ask permission unless the server marks the tool read-only. An MCP
// server is a program of its own: what it does happens outside the filter, and the UI says so.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"infuse/internal/core"
	"path/filepath"
	"strings"
	"sync"
)

const mcpProtocol = "2025-06-18"

// Protocol is the MCP revision this client speaks.
const Protocol = mcpProtocol

type ServerConfig struct {
	Type     string            `json:"type,omitempty"` // stdio (default) | http | sse
	Command  string            `json:"command,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	URL      string            `json:"url,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

type File struct {
	Servers map[string]ServerConfig `json:"mcpServers"`
}

type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnly bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

func FilePath(root, scope string) string {
	if scope == "global" {
		return filepath.Join(core.InfuseHome(), "mcp.json")
	}
	return filepath.Join(root, ".mcp.json")
}

func LoadFile(p string) File {
	var f File
	_ = core.ReadJSON(p, &f)
	if f.Servers == nil {
		f.Servers = map[string]ServerConfig{}
	}
	return f
}

func (c ServerConfig) IsHTTP() bool {
	return c.Type == "http" || c.Type == "sse" || (c.Command == "" && c.URL != "")
}

func (c ServerConfig) Describe() string {
	if c.IsHTTP() {
		return c.URL
	}
	return strings.TrimSpace(c.Command + " " + strings.Join(c.Args, " "))
}

// ---------------------------------------------------------------- approvals

var mcpApproveMu sync.Mutex

func mcpFingerprint(root, scope, name string, c ServerConfig) string {
	c.Disabled = false
	if scope == "global" {
		root = ""
	}
	b, _ := json.Marshal([]any{filepath.Clean(root), scope, name, c})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mcpApprovalsFile() string { return filepath.Join(core.InfuseHome(), "approved-mcp.json") }

func Approved(root, scope, name string, c ServerConfig) bool {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	var m map[string]bool
	_ = core.ReadJSON(mcpApprovalsFile(), &m)
	return m[mcpFingerprint(root, scope, name, c)]
}

func Approve(root, scope, name string, c ServerConfig) error {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	m := map[string]bool{}
	_ = core.ReadJSON(mcpApprovalsFile(), &m)
	m[mcpFingerprint(root, scope, name, c)] = true
	return core.WriteJSON(mcpApprovalsFile(), m)
}

// ---------------------------------------------------------------- client
