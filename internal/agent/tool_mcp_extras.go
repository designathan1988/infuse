package agent

// The agent's reach into what connectors offer besides tools: their resources and prompts. They are
// category "mcp": what a server returns goes through the permission policy and comes back marked as
// external content.

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/mcp"
	"strings"
)

// mcpExtraTool is one row of the table below.
type mcpExtraTool struct {
	name, description, inputSchema string
	run                            func(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, error)
}

func (t mcpExtraTool) Category() string { return "mcp" }
func (t mcpExtraTool) Spec() ToolSpec {
	return ToolSpec{Name: t.name, Description: t.description, InputSchema: schema(t.inputSchema)}
}

func (t mcpExtraTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	out, err := t.run(ctx, tc, input)
	if err != nil {
		return tc.SayErr(err), true
	}
	return tc.clip(externalContent("connector "+t.name, out)), false
}

const mcpServerArg = `"server":{"type":"string","description":"Connector name; omit to list every running connector"}`

var mcpExtraTools = []mcpExtraTool{
	{"ListMcpResources", "Lists the resources the running MCP connectors offer (uri, name, description).",
		`{"type":"object","properties":{` + mcpServerArg + `}}`, listMCPResources},
	{"ReadMcpResource", "Reads one resource of an MCP connector by its uri.",
		`{"type":"object","properties":{"server":{"type":"string"},"uri":{"type":"string"}},"required":["server","uri"]}`, readMCPResource},
	{"ListMcpPrompts", "Lists the prompts the running MCP connectors offer, with their arguments.",
		`{"type":"object","properties":{` + mcpServerArg + `}}`, listMCPPrompts},
	{"GetMcpPrompt", "Gets one prompt of an MCP connector, filled with its arguments.",
		`{"type":"object","properties":{"server":{"type":"string"},"name":{"type":"string"},"arguments":{"type":"object","additionalProperties":{"type":"string"}}},"required":["server","name"]}`, getMCPPrompt},
}

func mcpTools() []Tool {
	out := make([]Tool, 0, len(mcpExtraTools))
	for _, t := range mcpExtraTools {
		out = append(out, t)
	}
	return out
}

// mcpTargets are the connectors a listing covers.
func mcpTargets(tc *ToolContext, input json.RawMessage) ([]*mcp.Client, error) {
	var in struct{ Server string }
	_ = json.Unmarshal(input, &in)
	m := mcp.For(tc.S.Root)
	if in.Server == "" {
		return m.Running(), nil
	}
	c := m.Client(in.Server)
	if c == nil {
		return nil, errMsg(M("mcp.not_running", "name", in.Server))
	}
	return []*mcp.Client{c}, nil
}

func mcpClient(tc *ToolContext, name string) (*mcp.Client, error) {
	c := mcp.For(tc.S.Root).Client(name)
	if c == nil {
		return nil, errMsg(M("mcp.not_running", "name", name))
	}
	return c, nil
}

func listMCPResources(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, error) {
	clients, err := mcpTargets(tc, input)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range clients {
		list, err := c.Resources(ctx)
		if err != nil {
			fmt.Fprintf(&b, "%s: %v\n", c.Name(), err)
			continue
		}
		for _, r := range list {
			fmt.Fprintf(&b, "%s %s %s %s\n", c.Name(), r.URI, r.Name, r.Description)
		}
	}
	return firstNonEmpty(b.String(), tc.Say(M("mcp.nothing_listed"))), nil
}

func listMCPPrompts(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, error) {
	clients, err := mcpTargets(tc, input)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, c := range clients {
		list, err := c.Prompts(ctx)
		if err != nil {
			fmt.Fprintf(&b, "%s: %v\n", c.Name(), err)
			continue
		}
		for _, p := range list {
			fmt.Fprintf(&b, "%s %s: %s", c.Name(), p.Name, p.Description)
			for _, a := range p.Arguments {
				fmt.Fprintf(&b, " [%s]", a.Name)
			}
			b.WriteString("\n")
		}
	}
	return firstNonEmpty(b.String(), tc.Say(M("mcp.nothing_listed"))), nil
}

func readMCPResource(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, error) {
	var in struct{ Server, URI string }
	_ = json.Unmarshal(input, &in)
	c, err := mcpClient(tc, in.Server)
	if err != nil {
		return "", err
	}
	raw, err := c.ReadResource(ctx, in.URI)
	if err != nil {
		return "", err
	}
	var res struct {
		Contents []struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
			Blob string `json:"blob"`
			Mime string `json:"mimeType"`
		} `json:"contents"`
	}
	_ = json.Unmarshal(raw, &res)
	var b strings.Builder
	for _, item := range res.Contents {
		if item.Blob != "" {
			b.WriteString(tc.Say(M("mcp.resource_binary", "uri", item.URI, "type", item.Mime, "bytes", len(item.Blob)*3/4)) + "\n")
			continue
		}
		b.WriteString(item.Text + "\n")
	}
	return b.String(), nil
}

func getMCPPrompt(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, error) {
	var in struct {
		Server, Name string
		Arguments    map[string]string
	}
	_ = json.Unmarshal(input, &in)
	c, err := mcpClient(tc, in.Server)
	if err != nil {
		return "", err
	}
	raw, err := c.GetPrompt(ctx, in.Name, in.Arguments)
	if err != nil {
		return "", err
	}
	var res struct {
		Messages []struct {
			Role    string `json:"role"`
			Content struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(raw, &res)
	var b strings.Builder
	for _, m := range res.Messages {
		fmt.Fprintf(&b, "[%s] %s\n", m.Role, m.Content.Text)
	}
	return b.String(), nil
}
