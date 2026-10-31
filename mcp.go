package main

// MCP connectors (Model Context Protocol). Servers are declared like in Claude Code: .mcp.json in the
// project and ~/.infuse/mcp.json for every project, {"mcpServers": {name: {command, args, env} or
// {type: "http", url, headers}}}. A server runs only after the user approves that exact configuration
// (the agent can write .mcp.json; approvals live in ~/.infuse). Its tools reach the agent as
// mcp__<server>__<tool>; calls ask permission unless the server marks the tool read-only. An MCP
// server is a program of its own: what it does happens outside the filter, and the UI says so.

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const mcpProtocol = "2025-06-18"

type mcpServerConfig struct {
	Type     string            `json:"type,omitempty"` // stdio (default) | http | sse
	Command  string            `json:"command,omitempty"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	URL      string            `json:"url,omitempty"`
	Headers  map[string]string `json:"headers,omitempty"`
	Disabled bool              `json:"disabled,omitempty"`
}

type mcpFile struct {
	Servers map[string]mcpServerConfig `json:"mcpServers"`
}

type mcpTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Annotations struct {
		ReadOnly bool `json:"readOnlyHint"`
	} `json:"annotations"`
}

func mcpFilePath(root, scope string) string {
	if scope == "global" {
		return filepath.Join(InfuseHome(), "mcp.json")
	}
	return filepath.Join(root, ".mcp.json")
}

func loadMCPFile(p string) mcpFile {
	var f mcpFile
	_ = readJSON(p, &f)
	if f.Servers == nil {
		f.Servers = map[string]mcpServerConfig{}
	}
	return f
}

func (c mcpServerConfig) isHTTP() bool {
	return c.Type == "http" || c.Type == "sse" || (c.Command == "" && c.URL != "")
}

func (c mcpServerConfig) describe() string {
	if c.isHTTP() {
		return c.URL
	}
	return strings.TrimSpace(c.Command + " " + strings.Join(c.Args, " "))
}

// ---------------------------------------------------------------- approvals

var mcpApproveMu sync.Mutex

func mcpFingerprint(root, scope, name string, c mcpServerConfig) string {
	c.Disabled = false
	if scope == "global" {
		root = ""
	}
	b, _ := json.Marshal([]any{filepath.Clean(root), scope, name, c})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mcpApprovalsFile() string { return filepath.Join(InfuseHome(), "approved-mcp.json") }

func mcpApproved(root, scope, name string, c mcpServerConfig) bool {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	var m map[string]bool
	_ = readJSON(mcpApprovalsFile(), &m)
	return m[mcpFingerprint(root, scope, name, c)]
}

func approveMCP(root, scope, name string, c mcpServerConfig) error {
	mcpApproveMu.Lock()
	defer mcpApproveMu.Unlock()
	m := map[string]bool{}
	_ = readJSON(mcpApprovalsFile(), &m)
	m[mcpFingerprint(root, scope, name, c)] = true
	return writeJSON(mcpApprovalsFile(), m)
}

// ---------------------------------------------------------------- client

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type mcpClient struct {
	name, scope string
	cfg         mcpServerConfig
	nextID      int64

	mu      sync.Mutex
	state   string // starting | running | error | stopped
	err     string
	tools   []mcpTool
	stderr  bytes.Buffer
	pending map[int64]chan rpcMessage

	cmd   *exec.Cmd
	stdin io.WriteCloser

	httpc     *http.Client
	sessionID string
}

func (c *mcpClient) setErr(err error) {
	c.mu.Lock()
	c.state, c.err = "error", err.Error()
	c.mu.Unlock()
}

func (c *mcpClient) start(ctx context.Context, root string) error {
	c.mu.Lock()
	c.state, c.err, c.pending = "starting", "", map[int64]chan rpcMessage{}
	c.mu.Unlock()
	if c.cfg.isHTTP() {
		c.httpc = &http.Client{Timeout: 120 * time.Second}
	} else if err := c.spawn(root); err != nil {
		c.setErr(err)
		return err
	}
	init := map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "infuse", "version": version}}
	if _, err := c.call(ctx, "initialize", init); err != nil {
		c.setErr(fmt.Errorf("initialize: %w", err))
		c.close()
		return err
	}
	_ = c.notify(ctx, "notifications/initialized", nil)
	var tools []mcpTool
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.call(ctx, "tools/list", params)
		if err != nil {
			c.setErr(fmt.Errorf("tools/list: %w", err))
			c.close()
			return err
		}
		var res struct {
			Tools      []mcpTool `json:"tools"`
			NextCursor string    `json:"nextCursor"`
		}
		_ = json.Unmarshal(raw, &res)
		tools = append(tools, res.Tools...)
		if cursor = res.NextCursor; cursor == "" {
			break
		}
	}
	c.mu.Lock()
	c.tools, c.state = tools, "running"
	c.mu.Unlock()
	return nil
}

func (c *mcpClient) spawn(root string) error {
	if c.cfg.Command == "" {
		return errors.New("server has neither a command nor a url")
	}
	exe, err := exec.LookPath(c.cfg.Command)
	if err != nil {
		return fmt.Errorf("%s not found on this computer", c.cfg.Command)
	}
	args := make([]string, len(c.cfg.Args))
	for i, a := range c.cfg.Args {
		args[i] = os.ExpandEnv(a)
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = root
	cmd.Env = os.Environ()
	for k, v := range c.cfg.Env {
		cmd.Env = append(cmd.Env, k+"="+os.ExpandEnv(v))
	}
	noConsoleWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = &lockedWriter{mu: &c.mu, w: &c.stderr, max: 32 << 10}
	if err := cmd.Start(); err != nil {
		return err
	}
	c.cmd, c.stdin = cmd, stdin
	go c.readLoop(stdout)
	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		if c.state != "stopped" {
			c.state = "error"
			c.err = "the server exited"
			if err != nil {
				c.err += ": " + err.Error()
			}
		}
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()
	return nil
}

type lockedWriter struct {
	mu  *sync.Mutex
	w   *bytes.Buffer
	max int
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Write(p)
	if l.w.Len() > l.max {
		b := l.w.Bytes()
		keep := append([]byte{}, b[len(b)-l.max/2:]...)
		l.w.Reset()
		l.w.Write(keep)
	}
	return len(p), nil
}

func (c *mcpClient) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m rpcMessage
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		c.dispatch(m)
	}
}

// dispatch hands a response to its caller and answers the server's own requests (ping; others are refused).
func (c *mcpClient) dispatch(m rpcMessage) {
	if m.Method != "" {
		if m.ID != nil { // a request from the server
			reply := map[string]any{"jsonrpc": "2.0", "id": *m.ID}
			if m.Method == "ping" {
				reply["result"] = map[string]any{}
			} else {
				reply["error"] = map[string]any{"code": -32601, "message": "not supported by infuse"}
			}
			b, _ := json.Marshal(reply)
			c.writeLine(b)
		}
		return
	}
	if m.ID == nil {
		return
	}
	c.mu.Lock()
	ch := c.pending[*m.ID]
	delete(c.pending, *m.ID)
	c.mu.Unlock()
	if ch != nil {
		ch <- m
	}
}

func (c *mcpClient) writeLine(b []byte) {
	c.mu.Lock()
	w := c.stdin
	c.mu.Unlock()
	if w != nil {
		_, _ = w.Write(append(b, '\n'))
	}
}

func (c *mcpClient) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	p, _ := json.Marshal(params)
	req, _ := json.Marshal(rpcMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: p})
	var m rpcMessage
	if c.cfg.isHTTP() {
		var err error
		if m, err = c.post(ctx, req, id); err != nil {
			return nil, err
		}
	} else {
		ch := make(chan rpcMessage, 1)
		c.mu.Lock()
		if c.pending == nil {
			c.mu.Unlock()
			return nil, errors.New("server stopped")
		}
		c.pending[id] = ch
		c.mu.Unlock()
		c.writeLine(req)
		timeout := time.NewTimer(120 * time.Second)
		defer timeout.Stop()
		select {
		case r, ok := <-ch:
			if !ok {
				return nil, fmt.Errorf("the server exited: %s", c.stderrTail())
			}
			m = r
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, errors.New("the server did not answer within 120 s")
		}
	}
	if m.Error != nil {
		return nil, fmt.Errorf("%s (code %d)", m.Error.Message, m.Error.Code)
	}
	return m.Result, nil
}

func (c *mcpClient) notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	b, _ := json.Marshal(msg)
	if c.cfg.isHTTP() {
		_, err := c.post(ctx, b, 0)
		return err
	}
	c.writeLine(b)
	return nil
}

// post sends one message over Streamable HTTP; the answer comes as JSON or as an event stream.
func (c *mcpClient) post(ctx context.Context, body []byte, id int64) (rpcMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return rpcMessage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, os.ExpandEnv(v))
	}
	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()
	resp, err := c.httpc.Do(req)
	if err != nil {
		return rpcMessage{}, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}
	if id == 0 { // a notification
		return rpcMessage{}, nil
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return rpcMessage{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64<<10), 32<<20)
		var data strings.Builder
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data:") {
				data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				continue
			}
			if line == "" && data.Len() > 0 {
				var m rpcMessage
				if json.Unmarshal([]byte(data.String()), &m) == nil {
					if m.Method != "" {
						c.dispatch(m)
					} else if m.ID != nil && *m.ID == id {
						return m, nil
					}
				}
				data.Reset()
			}
		}
		return rpcMessage{}, errors.New("the server's stream ended without an answer")
	}
	var m rpcMessage
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return rpcMessage{}, err
	}
	return m, nil
}

func (c *mcpClient) stderrTail() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := strings.TrimSpace(c.stderr.String())
	if len(s) > 600 {
		s = s[len(s)-600:]
	}
	return s
}

func (c *mcpClient) close() {
	c.mu.Lock()
	if c.state != "error" {
		c.state = "stopped"
	}
	cmd, stdin := c.cmd, c.stdin
	c.cmd, c.stdin = nil, nil
	c.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		done := make(chan struct{})
		go func() { _, _ = cmd.Process.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = cmd.Process.Kill()
		}
	}
}

// ---------------------------------------------------------------- per-project manager

type mcpManager struct {
	root    string
	mu      sync.Mutex
	clients map[string]*mcpClient // scope/name
}

var (
	mcpManagers   = map[string]*mcpManager{}
	mcpManagersMu sync.Mutex
)

func mcpFor(root string) *mcpManager {
	mcpManagersMu.Lock()
	defer mcpManagersMu.Unlock()
	m := mcpManagers[root]
	if m == nil {
		m = &mcpManager{root: root, clients: map[string]*mcpClient{}}
		mcpManagers[root] = m
	}
	return m
}

type mcpEntry struct {
	Scope string
	Name  string
	Cfg   mcpServerConfig
}

// entries lists the servers of the project and the global ones (a project server overrides a global one
// with the same name).
func (m *mcpManager) entries() []mcpEntry {
	var out []mcpEntry
	proj := loadMCPFile(mcpFilePath(m.root, "project")).Servers
	for name, c := range loadMCPFile(mcpFilePath(m.root, "global")).Servers {
		if _, over := proj[name]; !over {
			out = append(out, mcpEntry{"global", name, c})
		}
	}
	for name, c := range proj {
		out = append(out, mcpEntry{"project", name, c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ensure starts the approved, enabled servers not running yet (in parallel) and stops the ones whose
// configuration changed or was removed.
func (m *mcpManager) ensure(ctx context.Context) {
	want := map[string]mcpEntry{}
	for _, e := range m.entries() {
		if !e.Cfg.Disabled && mcpApproved(m.root, e.Scope, e.Name, e.Cfg) {
			want[e.Scope+"/"+e.Name] = e
		}
	}
	var wg sync.WaitGroup
	m.mu.Lock()
	for key, c := range m.clients {
		e, ok := want[key]
		if !ok || mcpFingerprint(m.root, e.Scope, e.Name, e.Cfg) != mcpFingerprint(m.root, c.scope, c.name, c.cfg) {
			go c.close()
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
		c = &mcpClient{name: e.Name, scope: e.Scope, cfg: e.Cfg}
		m.clients[key] = c
		wg.Add(1)
		go func(c *mcpClient) {
			defer wg.Done()
			sctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()
			_ = c.start(sctx, m.root)
		}(c)
	}
	m.mu.Unlock()
	wg.Wait()
}

func (m *mcpManager) restart(ctx context.Context, scope, name string) {
	m.mu.Lock()
	if c := m.clients[scope+"/"+name]; c != nil {
		c.close()
		delete(m.clients, scope+"/"+name)
	}
	m.mu.Unlock()
	m.ensure(ctx)
}

var reToolName = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func mcpToolName(server, tool string) string {
	n := "mcp__" + reToolName.ReplaceAllString(server, "_") + "__" + reToolName.ReplaceAllString(tool, "_")
	if len(n) > 64 {
		n = n[:64]
	}
	return n
}

// tools returns the MCP tools of the running servers as agent tools.
func (m *mcpManager) tools() []Tool {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Tool
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
				out = append(out, mcpAgentTool{client: c, tool: t})
			}
		}
		c.mu.Unlock()
	}
	return out
}

type mcpAgentTool struct {
	client *mcpClient
	tool   mcpTool
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
	return ToolSpec{Name: mcpToolName(t.client.name, t.tool.Name), Description: fmt.Sprintf("[connector %s] %s", t.client.name, desc), InputSchema: schema}
}

func (t mcpAgentTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	if len(input) == 0 {
		input = json.RawMessage("{}")
	}
	Journal(tc.Root, JournalEntry{Kind: "mcp", Command: t.client.name + "/" + t.tool.Name, Detail: truncate(string(input), 300)})
	raw, err := t.client.call(ctx, "tools/call", map[string]any{"name": t.tool.Name, "arguments": input})
	if err != nil {
		return fmt.Sprintf("connector %s: %s", t.client.name, err.Error()), true
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
				tc.S.mu.Lock()
				tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: c.MimeType, Data: c.Data})
				tc.S.mu.Unlock()
				b.WriteString("[image attached to this reply]\n")
			}
		case "resource":
			b.WriteString(string(c.Resource) + "\n")
		}
	}
	if b.Len() == 0 && len(res.Structured) > 0 {
		b.Write(res.Structured)
	}
	return clip(externalContent(fmt.Sprintf("connector %s/%s", t.client.name, t.tool.Name), strings.TrimSpace(b.String()))), res.IsError
}

// sessionTools is the session's tool set: the built-in tools and the running connectors' tools.
func (s *Session) sessionTools() map[string]Tool {
	out := map[string]Tool{}
	for k, v := range toolRegistry {
		out[k] = v
	}
	for _, t := range mcpFor(s.Root).tools() {
		out[t.Spec().Name] = t
	}
	return out
}

func (s *Session) toolSpecs() []ToolSpec {
	specs := toolSpecs()
	for _, t := range mcpFor(s.Root).tools() {
		specs = append(specs, t.Spec())
	}
	return specs
}

func (s *Session) lookupTool(name string) (Tool, bool) {
	if t, ok := toolRegistry[name]; ok {
		return t, true
	}
	if strings.HasPrefix(name, "mcp__") {
		for _, t := range mcpFor(s.Root).tools() {
			if t.Spec().Name == name {
				return t, true
			}
		}
	}
	return nil, false
}

// ---------------------------------------------------------------- status for the UI

type mcpStatus struct {
	Scope    string          `json:"scope"`
	Name     string          `json:"name"`
	Target   string          `json:"target"` // command line or URL
	HTTP     bool            `json:"http"`
	Approved bool            `json:"approved"`
	Disabled bool            `json:"disabled"`
	State    string          `json:"state"`
	Error    string          `json:"error,omitempty"`
	Stderr   string          `json:"stderr,omitempty"`
	Tools    []mcpToolStatus `json:"tools"`
	Config   mcpServerConfig `json:"config"`
}

type mcpToolStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ReadOnly    bool   `json:"readOnly"`
}

func (m *mcpManager) status() []mcpStatus {
	out := []mcpStatus{}
	for _, e := range m.entries() {
		st := mcpStatus{Scope: e.Scope, Name: e.Name, Target: e.Cfg.describe(), HTTP: e.Cfg.isHTTP(), Approved: mcpApproved(m.root, e.Scope, e.Name, e.Cfg), Disabled: e.Cfg.Disabled, State: "stopped", Tools: []mcpToolStatus{}, Config: e.Cfg}
		m.mu.Lock()
		c := m.clients[e.Scope+"/"+e.Name]
		m.mu.Unlock()
		if c != nil {
			c.mu.Lock()
			st.State, st.Error = c.state, c.err
			for _, t := range c.tools {
				d := t.Description
				if len(d) > 300 {
					d = d[:300] + "…"
				}
				st.Tools = append(st.Tools, mcpToolStatus{Name: t.Name, Description: d, ReadOnly: t.Annotations.ReadOnly})
			}
			c.mu.Unlock()
			if st.State == "error" {
				st.Stderr = c.stderrTail()
			}
		}
		out = append(out, st)
	}
	return out
}

// editMCP changes one scope's file (add, replace, remove or toggle a server).
func editMCP(root, scope string, f func(map[string]mcpServerConfig)) error {
	p := mcpFilePath(root, scope)
	file := loadMCPFile(p)
	f(file.Servers)
	return writeJSON(p, file)
}

// closeAllMCP stops every connector process (the app is exiting).
func closeAllMCP() {
	mcpManagersMu.Lock()
	defer mcpManagersMu.Unlock()
	for _, m := range mcpManagers {
		m.mu.Lock()
		for k, c := range m.clients {
			c.close()
			delete(m.clients, k)
		}
		m.mu.Unlock()
	}
}
