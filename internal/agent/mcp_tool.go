package agent

// The agent-facing side of MCP: connector tools exposed as mcp__<server>__<tool>, and the session's
// tool set (built-ins plus running connectors). The protocol and lifecycle live in internal/mcp.

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/mcp"
	"strings"
)

type mcpAgentTool struct {
	client *mcp.Client
	tool   mcp.ToolDef
}

func (t mcpAgentTool) Category() string {
	if t.tool.Annotations.ReadOnly {
		return "read"
	}
	return "mcp"
}

func (t mcpAgentTool) Spec() ToolSpec {
	schema := t.tool.InputSchema
	if len(schema) == 0 || string(schema) == "null" {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}
	desc := t.tool.Description
	if len(desc) > 1500 {
		desc = desc[:1500] + "…"
	}
	return ToolSpec{Name: mcp.ToolName(t.client.Name(), t.tool.Name), Description: fmt.Sprintf("[connector %s] %s", t.client.Name(), desc), InputSchema: schema}
}

func (t mcpAgentTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	Journal(tc.Root, JournalEntry{Kind: "mcp", Command: t.client.Name() + "/" + t.tool.Name, Detail: truncate(string(input), 300)})
	raw, err := t.client.Call(ctx, "tools/call", map[string]any{"name": t.tool.Name, "arguments": input})
	if err != nil {
		return tc.Say(M("mcp.call_failed", "name", t.client.Name(), "error", errArg(err))), true
	}
	var res struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			Data     string          `json:"data"`
			MimeType string          `json:"mimeType"`
			Resource json.RawMessage `json:"resource"`
		} `json:"content"`
		Structured json.RawMessage `json:"structuredContent"`
		IsError    bool            `json:"isError"`
	}
	_ = json.Unmarshal(raw, &res)
	var b strings.Builder
	for _, c := range res.Content {
		switch c.Type {
		case "text":
			b.WriteString(c.Text + "\n")
		case "image":
			if len(c.Data) < 7<<20 {
				tc.S.Mu.Lock()
				tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: c.MimeType, Data: c.Data})
				tc.S.Mu.Unlock()
				b.WriteString(tc.Say(M("mcp.image_attached")) + "\n")
			}
		case "resource":
			b.WriteString(string(c.Resource) + "\n")
		}
	}
	if b.Len() == 0 && len(res.Structured) > 0 {
		b.Write(res.Structured)
	}
	return tc.clip(externalContent(fmt.Sprintf("connector %s/%s", t.client.Name(), t.tool.Name), strings.TrimSpace(b.String()))), res.IsError
}

// sessionTools is the session's tool set: the built-in tools and the running connectors' tools.
func (s *Session) sessionTools() map[string]Tool {
	out := map[string]Tool{}
	for k, v := range toolRegistry {
		out[k] = v
	}
	for _, t := range mcp.For(s.Root).Tools() {
		agent := mcpAgentTool{client: t.Client, tool: t.Tool}
		out[agent.Spec().Name] = agent
	}
	return out
}

func (s *Session) toolSpecs() []ToolSpec {
	specs := toolSpecs()
	for _, t := range mcp.For(s.Root).Tools() {
		agent := mcpAgentTool{client: t.Client, tool: t.Tool}
		specs = append(specs, agent.Spec())
	}
	return specs
}

func (s *Session) lookupTool(name string) (Tool, bool) {
	if t, ok := toolRegistry[name]; ok {
		return t, true
	}
	if strings.HasPrefix(name, "mcp__") {
		for _, t := range mcp.For(s.Root).Tools() {
			agent := mcpAgentTool{client: t.Client, tool: t.Tool}
			if agent.Spec().Name == name {
				return agent, true
			}
		}
	}
	return nil, false
}
