package mcp

import (
	"context"
	"regexp"
	"sort"
	"sync"
	"time"
)

// MCP connectors: configuration, protocol client and per-project manager.
// Per-project manager, lifecycle and the agent-facing tool list.

type Manager struct {
	root    string
	mu      sync.Mutex
	clients map[string]*Client // scope/name
}

var (
	Managers   = map[string]*Manager{}
	ManagersMu sync.Mutex
)

func For(root string) *Manager {
	ManagersMu.Lock()
	defer ManagersMu.Unlock()
	m := Managers[root]
	if m == nil {
		m = &Manager{root: root, clients: map[string]*Client{}}
		Managers[root] = m
	}
	return m
}

type Entry struct {
	Scope string
	Name  string
	Cfg   ServerConfig
}

// entries lists the servers of the project and the global ones (a project server overrides a global one
// with the same name).
func (m *Manager) Entries() []Entry {
	var out []Entry
	proj := LoadFile(FilePath(m.root, "project")).Servers
	for name, c := range LoadFile(FilePath(m.root, "global")).Servers {
		if _, over := proj[name]; !over {
			out = append(out, Entry{"global", name, c})
		}
	}
	for name, c := range proj {
		out = append(out, Entry{"project", name, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ensure starts the approved, enabled servers not running yet (in parallel) and stops the ones whose
// configuration changed or was removed.
func (m *Manager) Ensure(ctx context.Context) {
	want := map[string]Entry{}
	for _, e := range m.Entries() {
		if !e.Cfg.Disabled && Approved(m.root, e.Scope, e.Name, e.Cfg) {
			want[e.Scope+"/"+e.Name] = e
		}
	}
	var wg sync.WaitGroup
	m.mu.Lock()
	for key, c := range m.clients {
		e, ok := want[key]
		if !ok || mcpFingerprint(m.root, e.Scope, e.Name, e.Cfg) != mcpFingerprint(m.root, c.scope, c.name, c.cfg) {
			go c.Close()
			delete(m.clients, key)
		}
	}
	for key, e := range want {
		c := m.clients[key]
		if c != nil {
			c.mu.Lock()
			st := c.state
			c.mu.Unlock()
			if st == "running" || st == "starting" || st == "error" {
				continue // an error stays until the user restarts it
			}
		}
		c = &Client{name: e.Name, scope: e.Scope, cfg: e.Cfg}
		m.clients[key] = c
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			_ = c.Start(sctx, m.root)
		}(c)
	}
	m.mu.Unlock()
	wg.Wait()
}

func (m *Manager) Restart(ctx context.Context, scope, name string) {
	m.mu.Lock()
	if c := m.clients[scope+"/"+name]; c != nil {
		c.Close()
		delete(m.clients, scope+"/"+name)
	}
	m.mu.Unlock()
	m.Ensure(ctx)
}

var reToolName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func ToolName(server, tool string) string {
	n := "mcp__" + reToolName.ReplaceAllString(server, "_") + "__" + reToolName.ReplaceAllString(tool, "_")
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

// tools returns the MCP tools of the running servers as agent tools.
type AgentTool struct {
	Client *Client
	Tool   ToolDef
}

// Name is the server name.
func (c *Client) Name() string { return c.name }

func (m *Manager) Tools() []AgentTool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []AgentTool
	keys := make([]string, 0, len(m.clients))
	for k := range m.clients {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		c := m.clients[k]
		c.mu.Lock()
		if c.state == "running" {
			for _, t := range c.tools {
				out = append(out, AgentTool{Client: c, Tool: t})
			}
		}
		c.mu.Unlock()
	}
	return out
}

// ---------------------------------------------------------------- status for the UI
