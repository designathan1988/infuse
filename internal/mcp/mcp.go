package mcp

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
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/platform/process"
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

type RPCMessage struct {
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

// NewClient builds a client for a server configuration (tests and explicit wiring).
func NewClient(name, scope string, cfg ServerConfig) *Client {
	return &Client{name: name, scope: scope, cfg: cfg}
}

type Client struct {
	name, scope string
	cfg         ServerConfig
	nextID      int64

	mu      sync.Mutex
	state   string // starting | running | error | stopped
	err     core.Msg    // why it is in error (written in the reader's language when shown)
	tools   []ToolDef
	stderr  bytes.Buffer
	pending map[int64]chan RPCMessage

	cmd   *exec.Cmd
	stdin io.WriteCloser

	httpc     *http.Client
	sessionID string
}

func (c *Client) SetErr(m core.Msg) {
	c.mu.Lock()
	c.state, c.err = "error", m
	c.mu.Unlock()
}

func (c *Client) Start(ctx context.Context, root string) error {
	c.mu.Lock()
	c.state, c.err, c.pending = "starting", core.Msg{}, map[int64]chan RPCMessage{}
	c.mu.Unlock()
	if c.cfg.IsHTTP() {
		c.httpc = &http.Client{Timeout: 120 * time.Second}
	} else if err := c.Spawn(root); err != nil {
		c.SetErr(core.M("mcp.start_failed", "error", core.ErrArg(err)))
		return err
	}
	init := map[string]any{"protocolVersion": mcpProtocol, "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "infuse", "core.Version": core.Version}}
	if _, err := c.Call(ctx, "initialize", init); err != nil {
		c.SetErr(core.M("mcp.step_failed", "step", "initialize", "error", core.ErrArg(err)))
		c.Close()
		return err
	}
	_ = c.Notify(ctx, "notifications/initialized", nil)
	var tools []ToolDef
	cursor := ""
	for page := 0; page < 20; page++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		raw, err := c.Call(ctx, "tools/list", params)
		if err != nil {
			c.SetErr(core.M("mcp.step_failed", "step", "tools/list", "error", core.ErrArg(err)))
			c.Close()
			return err
		}
		var res struct {
			Tools      []ToolDef `json:"tools"`
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

func (c *Client) Spawn(root string) error {
	if c.cfg.Command == "" {
		return core.ErrMsg(core.M("mcp.no_command"))
	}
	exe, err := exec.LookPath(c.cfg.Command)
	if err != nil {
		return core.ErrMsg(core.M("mcp.not_found", "command", c.cfg.Command))
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
	process.NoConsoleWindow(cmd)
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
	go c.ReadLoop(stdout)
	go func() {
		err := cmd.Wait()
		c.mu.Lock()
		if c.state != "stopped" {
			c.state = "error"
			c.err = core.M("mcp.exited")
			if err != nil {
				c.err = core.M("mcp.exited_err", "error", err.Error())
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

func (c *Client) ReadLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var m RPCMessage
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		c.Dispatch(m)
	}
}

// dispatch hands a response to its caller and answers the server's own requests (ping; others are refused).
func (c *Client) Dispatch(m RPCMessage) {
	if m.Method != "" {
		if m.ID != nil { // a request from the server
			reply := map[string]any{"jsonrpc": "2.0", "id": *m.ID}
			if m.Method == "ping" {
				reply["result"] = map[string]any{}
			} else {
				reply["error"] = map[string]any{"code": -32601, "message": "not supported by infuse"}
			}
			b, _ := json.Marshal(reply)
			c.WriteLine(b)
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

func (c *Client) WriteLine(b []byte) {
	c.mu.Lock()
	w := c.stdin
	c.mu.Unlock()
	if w != nil {
		_, _ = w.Write(append(b, '\n'))
	}
}

func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&c.nextID, 1)
	p, _ := json.Marshal(params)
	req, _ := json.Marshal(RPCMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: p})
	var m RPCMessage
	if c.cfg.IsHTTP() {
		var err error
		if m, err = c.Post(ctx, req, id); err != nil {
			return nil, err
		}
	} else {
		ch := make(chan RPCMessage, 1)
		c.mu.Lock()
		if c.pending == nil {
			c.mu.Unlock()
			return nil, core.ErrMsg(core.M("mcp.stopped"))
		}
		c.pending[id] = ch
		c.mu.Unlock()
		c.WriteLine(req)
		timeout := time.NewTimer(120 * time.Second)
		defer timeout.Stop()
		select {
		case r, ok := <-ch:
			if !ok {
				return nil, core.ErrMsg(core.M("mcp.exited_err", "error", c.StderrTail()))
			}
			m = r
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout.C:
			return nil, core.ErrMsg(core.M("mcp.timeout"))
		}
	}
	if m.Error != nil {
		return nil, fmt.Errorf("%s (code %d)", m.Error.Message, m.Error.Code)
	}
	return m.Result, nil
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	b, _ := json.Marshal(msg)
	if c.cfg.IsHTTP() {
		_, err := c.Post(ctx, b, 0)
		return err
	}
	c.WriteLine(b)
	return nil
}

// post sends one message over Streamable HTTP; the answer comes as JSON or as an event stream.
func (c *Client) Post(ctx context.Context, body []byte, id int64) (RPCMessage, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return RPCMessage{}, err
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
		return RPCMessage{}, err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		c.mu.Lock()
		c.sessionID = sid
		c.mu.Unlock()
	}
	if id == 0 { // a notification
		return RPCMessage{}, nil
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return RPCMessage{}, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
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
				var m RPCMessage
				if json.Unmarshal([]byte(data.String()), &m) == nil {
					if m.Method != "" {
						c.Dispatch(m)
					} else if m.ID != nil && *m.ID == id {
						return m, nil
					}
				}
				data.Reset()
			}
		}
		return RPCMessage{}, core.ErrMsg(core.M("mcp.stream_ended"))
	}
	var m RPCMessage
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return RPCMessage{}, err
	}
	return m, nil
}

func (c *Client) StderrTail() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := strings.TrimSpace(c.stderr.String())
	if len(s) > 600 {
		s = s[len(s)-600:]
	}
	return s
}

func (c *Client) Close() {
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
	Config   ServerConfig `json:"config"`
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
