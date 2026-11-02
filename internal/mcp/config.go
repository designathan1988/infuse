package mcp

// MCP connectors (Model Context Protocol). Servers are declared like in Claude Code: .mcp.json in the
// project and ~/.infuse/mcp.json for every project, {"mcpServers": {name: {command, args, env} or
// {type: "http", url, headers}}}. A server runs only after the user approves that exact configuration
// (the agent can write .mcp.json; approvals live in ~/.infuse). Its tools reach the agent as
// mcp__<server>__<tool>; calls ask permission unless the server marks the tool read-only. An MCP
// server is a program of its own: what it does happens outside the filter, and the UI says so.
//
// The approval covers what runs, not only how it is named: a script inside the project is part of the
// fingerprint, so an approved "node tools/mcp.js" whose file changed is a server awaiting approval. A
// server runs contained like the agent's shell, whoever declared it.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/filter"
	"infuse/internal/sandbox"
	"io"
	"os"
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
	ClientID string            `json:"clientId,omitempty"` // OAuth client, when the server has no registration
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
	launched := launchedScripts(root, c)
	if scope == "global" {
		root = ""
	}
	b, _ := json.Marshal([]any{filepath.Clean(root), scope, name, c, launched})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// launchedScripts hashes the project files a stdio server's command line names: the program itself or
// a script handed to an interpreter. Each argument is tried as a path from the project root, where the
// server starts; a flag's value counts too (--entry=tools/mcp.js).
func launchedScripts(root string, c ServerConfig) map[string]string {
	if c.IsHTTP() {
		return nil
	}
	out := map[string]string{}
	for _, raw := range append([]string{c.Command}, c.Args...) {
		arg := os.ExpandEnv(raw)
		if _, value, isFlag := strings.Cut(arg, "="); isFlag {
			arg = value
		}
		if rel, sum := projectFileHash(root, arg); rel != "" {
			out[rel] = sum
		}
	}
	return out
}

// projectFileHash returns the file's project-relative path and content hash when arg names a regular
// file inside root ("" otherwise).
func projectFileHash(root, arg string) (string, string) {
	if arg == "" || strings.ContainsRune(arg, 0) {
		return "", ""
	}
	abs := arg
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ""
	}
	f, err := os.Open(abs)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return "", ""
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return filepath.ToSlash(rel), "unreadable"
	}
	return filepath.ToSlash(rel), hex.EncodeToString(h.Sum(nil))
}

// containedServer is how every stdio server starts, whoever declared it: an MCP server is an agent
// process, and a tool it runs reaches the project as much as the shell does.
func containedServer(root string, argv []string) []string {
	settings := config.EffectiveSettings(root)
	return sandbox.Wrap(argv, root, filter.ContainedRecords(settings.ProtectedPaths), settings.ContainProcesses)
}

func mcpApprovalsFile() string { return filepath.Join(core.InfuseHome(), "approved-mcp.json") }

func Approved(root, scope, name string, c ServerConfig) bool {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	var m map[string]bool
	// The file is signed by the user's key: an unsigned or edited file approves nothing.
	if err := config.OpenJSON(mcpApprovalsFile(), &m); err != nil {
		return false
	}
	return m[mcpFingerprint(root, scope, name, c)]
}

func Approve(root, scope, name string, c ServerConfig) error {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	m := map[string]bool{}
	_ = config.OpenJSON(mcpApprovalsFile(), &m)
	m[mcpFingerprint(root, scope, name, c)] = true
	return config.SealJSON(mcpApprovalsFile(), m)
}

// ---------------------------------------------------------------- client
