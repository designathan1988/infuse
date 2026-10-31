package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestMain doubles as a fake stdio MCP server when INFUSE_FAKE_MCP is set (the test binary runs itself).
func TestMain(m *testing.M) {
	if os.Getenv("INFUSE_FAKE_MCP") == "1" {
		fakeMCPStdio(os.Stdin, os.Stdout)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeMCPReply(req rpcMessage) map[string]any {
	switch req.Method {
	case "initialize":
		return map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{"tools": map[string]any{}}, "serverInfo": map[string]any{"name": "fake", "version": "1"}}
	case "tools/list":
		return map[string]any{"tools": []any{
			map[string]any{"name": "echo", "description": "repete o texto", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}}, "annotations": map[string]any{"readOnlyHint": true}},
			map[string]any{"name": "apaga tudo", "description": "perigoso", "inputSchema": map[string]any{"type": "object"}},
		}}
	case "tools/call":
		var p struct {
			Name      string            `json:"name"`
			Arguments map[string]string `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &p)
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": "eco: " + p.Arguments["text"]}}}
	}
	return nil
}

func fakeMCPStdio(in io.Reader, out io.Writer) {
	sc := bufio.NewScanner(in)
	for sc.Scan() {
		var req rpcMessage
		if json.Unmarshal(sc.Bytes(), &req) != nil || req.ID == nil {
			continue
		}
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": fakeMCPReply(req)})
		fmt.Fprintf(out, "%s\n", b)
	}
}

func TestMCPStdioServerNeedsApprovalThenServesTools(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	exe, _ := os.Executable()
	cfg := mcpServerConfig{Command: exe, Args: []string{"-test.run=^$"}, Env: map[string]string{"INFUSE_FAKE_MCP": "1"}}
	if err := editMCP(root, "project", func(sv map[string]mcpServerConfig) { sv["fake"] = cfg }); err != nil {
		t.Fatal(err)
	}
	m := mcpFor(root)
	defer m.restart(context.Background(), "project", "fake")
	m.ensure(context.Background())
	if len(m.tools()) != 0 {
		t.Fatal("an unapproved server started")
	}
	if st := m.status(); len(st) != 1 || st[0].Approved {
		t.Fatalf("%+v", st)
	}
	_ = approveMCP(root, "project", "fake", cfg)
	m.ensure(context.Background())
	tools := m.tools()
	if len(tools) != 2 {
		t.Fatalf("tools: %d %+v", len(tools), m.status())
	}
	s := newSession(root)
	echo, ok := s.lookupTool("mcp__fake__echo")
	if !ok || echo.Category() != "read" {
		t.Fatal("echo tool")
	}
	if danger, ok := s.lookupTool("mcp__fake__apaga_tudo"); !ok || danger.Category() != "mcp" {
		t.Fatal("a tool not marked read-only must ask")
	}
	out, isErr := echo.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"text":"oi"}`))
	if isErr || !strings.Contains(out, "eco: oi") || !strings.Contains(out, "external-content") {
		t.Fatal(out)
	}
	found := false
	for _, sp := range s.toolSpecs() {
		found = found || sp.Name == "mcp__fake__echo"
	}
	if !found {
		t.Fatal("connector tool not offered to the model")
	}
	// a changed configuration needs approval again
	_ = editMCP(root, "project", func(sv map[string]mcpServerConfig) { c := sv["fake"]; c.Args = append(c.Args, "-x"); sv["fake"] = c })
	m.ensure(context.Background())
	if len(m.tools()) != 0 {
		t.Fatal("a changed configuration ran without a new approval")
	}
}

func TestMCPStreamableHTTP(t *testing.T) {
	testHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req rpcMessage
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			w.WriteHeader(202)
			return
		}
		if req.Method != "initialize" && r.Header.Get("Mcp-Session-Id") != "s1" {
			http.Error(w, "missing session", 400)
			return
		}
		w.Header().Set("Mcp-Session-Id", "s1")
		b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": fakeMCPReply(req)})
		if req.Method == "tools/call" { // answer as an event stream, with a notification first
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "data: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\"}\n\ndata: %s\n\n", b)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	c := &mcpClient{name: "remoto", scope: "global", cfg: mcpServerConfig{Type: "http", URL: srv.URL}}
	if err := c.start(context.Background(), t.TempDir()); err != nil {
		t.Fatal(err)
	}
	raw, err := c.call(context.Background(), "tools/call", map[string]any{"name": "echo", "arguments": map[string]string{"text": "web"}})
	if err != nil || !strings.Contains(string(raw), "eco: web") {
		t.Fatal(err, string(raw))
	}
}
