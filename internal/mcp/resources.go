package mcp

// Resources and prompts: what a server offers besides tools. Calling these on a server that does not
// offer them comes back as the server's own error.

import (
	"context"
	"encoding/json"
	"sort"
)

// Resource is one entry of resources/list.
type Resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

// PromptArg is an argument a prompt takes.
type PromptArg struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// Prompt is one entry of prompts/list.
type Prompt struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Arguments   []PromptArg `json:"arguments,omitempty"`
}

// pageLimit bounds the pages followed through nextCursor.
const pageLimit = 20

// listAll follows a paginated list, decoding each page's items under key.
func listAll[T any](ctx context.Context, c *Client, method, key string) ([]T, error) {
	var out []T
	cursor := ""
	for range pageLimit {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.Call(ctx, method, params)
		if err != nil {
			return out, err
		}
		var page map[string]json.RawMessage
		if err := json.Unmarshal(raw, &page); err != nil {
			return out, err
		}
		var items []T
		_ = json.Unmarshal(page[key], &items)
		out = append(out, items...)
		cursor = ""
		_ = json.Unmarshal(page["nextCursor"], &cursor)
		if cursor == "" {
			break
		}
	}
	return out, nil
}

// Resources lists the server's resources.
func (c *Client) Resources(ctx context.Context) ([]Resource, error) {
	return listAll[Resource](ctx, c, "resources/list", "resources")
}

// Prompts lists the server's prompts.
func (c *Client) Prompts(ctx context.Context) ([]Prompt, error) {
	return listAll[Prompt](ctx, c, "prompts/list", "prompts")
}

// ReadResource returns the raw result of resources/read.
func (c *Client) ReadResource(ctx context.Context, uri string) (json.RawMessage, error) {
	return c.Call(ctx, "resources/read", map[string]any{"uri": uri})
}

// GetPrompt returns the raw result of prompts/get.
func (c *Client) GetPrompt(ctx context.Context, name string, args map[string]string) (json.RawMessage, error) {
	return c.Call(ctx, "prompts/get", map[string]any{"name": name, "arguments": args})
}

// Running lists the running servers' clients, by name.
func (m *Manager) Running() []*Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []*Client
	for _, c := range m.clients {
		c.mu.Lock()
		if c.state == "running" {
			out = append(out, c)
		}
		c.mu.Unlock()
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// Client finds a running server by name.
func (m *Manager) Client(name string) *Client {
	for _, c := range m.Running() {
		if c.name == name {
			return c
		}
	}
	return nil
}
