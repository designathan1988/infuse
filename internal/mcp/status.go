package mcp

import (
	"infuse/internal/config"
	"infuse/internal/core"
)

// MCP connectors: configuration, protocol client and per-project manager.
// Connection and tool status for the UI.

type Status struct {
	Scope    string          `json:"scope"`
	Name     string          `json:"name"`
	Target   string          `json:"target"` // command line or URL
	HTTP     bool            `json:"http"`
	Approved bool            `json:"approved"`
	Disabled bool            `json:"disabled"`
	State    string          `json:"state"`
	Error    string          `json:"error,omitempty"`
	Stderr   string          `json:"stderr,omitempty"`
	Tools    []ToolDefStatus `json:"tools"`
	Config   ServerConfig    `json:"config"`
}

type ToolDefStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
}

func (m *Manager) Status() []Status {
	out := []Status{}
	lang := config.RootLanguage(m.root)
	for _, e := range m.Entries() {
		st := Status{Scope: e.Scope, Name: e.Name, Target: e.Cfg.Describe(), HTTP: e.Cfg.IsHTTP(), Approved: Approved(m.root, e.Scope, e.Name, e.Cfg), Disabled: e.Cfg.Disabled, State: "stopped", Tools: []ToolDefStatus{}, Config: e.Cfg}
		m.mu.Lock()
		c := m.clients[e.Scope+"/"+e.Name]
		m.mu.Unlock()
		if c != nil {
			c.mu.Lock()
			st.State, st.Error = c.state, c.err.Render(lang)
			for _, t := range c.tools {
				d := t.Description
				if len(d) > 300 {
					d = d[:300] + "…"
				}
				st.Tools = append(st.Tools, ToolDefStatus{Name: t.Name, Description: d, ReadOnly: t.Annotations.ReadOnly})
			}
			c.mu.Unlock()
			if st.State == "error" {
				st.Stderr = c.StderrTail()
			}
		}
		out = append(out, st)
	}
	return out
}

// Edit changes one scope's file (add, replace, remove or toggle a server).
func Edit(root, scope string, f func(map[string]ServerConfig)) error {
	p := FilePath(root, scope)
	file := LoadFile(p)
	f(file.Servers)
	return core.WriteJSON(p, file)
}

// CloseAll stops every connector process (the app is exiting).
func CloseAll() {
	ManagersMu.Lock()
	defer ManagersMu.Unlock()
	for _, m := range Managers {
		m.mu.Lock()
		for k, c := range m.clients {
			c.Close()
			delete(m.clients, k)
		}
		m.mu.Unlock()
	}
}
