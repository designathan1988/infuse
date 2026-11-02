package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCP connectors: configuration, protocol client and per-project manager.
// JSON-RPC client over stdio and streamable HTTP.

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
	state   string   // starting | running | error | stopped
	err     core.Msg // why it is in error (written in the reader's language when shown)
	tools   []ToolDef
	stderr  bytes.Buffer
	pending map[int64]chan RPCMessage

	cmd   *exec.Cmd
	stdin io.WriteCloser

	httpc     *http.Client
	sessionID string
	root      string // the project, for the server's login in the vault
}

func (c *Client) SetErr(m core.Msg) {
	c.mu.Lock()
	c.state, c.err = "error", m
	c.mu.Unlock()
}

func (c *Client) Start(ctx context.Context, root string) error {
	c.mu.Lock()
	c.state, c.err, c.pending = "starting", core.Msg{}, map[int64]chan RPCMessage{}
	c.root = root
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
	argv := containedServer(root, append([]string{exe}, args...))
	cmd := exec.Command(argv[0], argv[1:]...)
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
	process.AttachJob(cmd)
	c.cmd, c.stdin = cmd, stdin
	go c.ReadLoop(stdout)
	go func() {
		err := cmd.Wait()
		process.ReleaseJob(cmd)
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
	req, err := c.newPostRequest(ctx, body)
	if err != nil {
		return RPCMessage{}, err
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return RPCMessage{}, err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.retryAuth(ctx) { // an expired login, once
		resp.Body.Close()
		if req, err = c.newPostRequest(ctx, body); err != nil {
			return RPCMessage{}, err
		}
		if resp, err = c.httpc.Do(req); err != nil {
			return RPCMessage{}, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return RPCMessage{}, core.ErrMsg(core.M("mcp.oauth_login_needed", "name", c.name))
	}
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
		return c.readEventStream(resp.Body, id)
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
