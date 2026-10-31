package main

// A headless Chromium-family browser (Edge, Chrome, Chromium — whichever is installed) driven over the
// DevTools protocol, for the agent's Browser tool. It never shows a window. One browser per project,
// with its own profile under ~/.infuse, started on first use and closed after 15 idle minutes.

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- minimal WebSocket client (RFC 6455)

type wsConn struct {
	c  net.Conn
	br *bufio.Reader
	mu sync.Mutex
}

func wsDial(raw string) (*wsConn, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("tcp", u.Host, 10*time.Second)
	if err != nil {
		return nil, err
	}
	keyb := make([]byte, 16)
	_, _ = rand.Read(keyb)
	key := base64.StdEncoding.EncodeToString(keyb)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", u.RequestURI(), u.Host, key)
	if _, err := c.Write([]byte(req)); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.StatusCode != 101 || resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		c.Close()
		return nil, fmt.Errorf("websocket: resposta %d", resp.StatusCode)
	}
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) writeText(p []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	hdr := []byte{0x81}
	n := len(p)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n)|0x80)
	case n < 65536:
		hdr = append(hdr, 126|0x80, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127|0x80)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	hdr = append(hdr, mask...)
	out := make([]byte, n)
	for i := range p {
		out[i] = p[i] ^ mask[i%4]
	}
	_, err := w.c.Write(append(hdr, out...))
	return err
}

// readMessage returns the next complete text or binary message (control frames handled here).
func (w *wsConn) readMessage() ([]byte, error) {
	var msg []byte
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(w.br, h); err != nil {
			return nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0f
		n := int64(h[1] & 0x7f)
		switch n {
		case 126:
			b := make([]byte, 2)
			if _, err := io.ReadFull(w.br, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint16(b))
		case 127:
			b := make([]byte, 8)
			if _, err := io.ReadFull(w.br, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint64(b))
		}
		if h[1]&0x80 != 0 { // servers do not mask; skip a mask if present
			if _, err := io.CopyN(io.Discard, w.br, 4); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9: // ping → pong
			w.mu.Lock()
			_, _ = w.c.Write([]byte{0x8A, 0x80, 0, 0, 0, 0})
			w.mu.Unlock()
			continue
		case 0xA:
			continue
		}
		msg = append(msg, payload...)
		if fin {
			return msg, nil
		}
	}
}

// ---------------------------------------------------------------- CDP session

type cdpBrowser struct {
	profile string
	cmd     *exec.Cmd
	ws      *wsConn
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan json.RawMessage
	errs    map[int64]string
	console []string
	lastUse time.Time
	closed  bool
}

func findChromium() string {
	var cands []string
	switch runtime.GOOS {
	case "windows":
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				cands = append(cands, filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`), filepath.Join(base, `Google\Chrome\Application\chrome.exe`))
			}
		}
	case "darwin":
		cands = []string{"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "/Applications/Chromium.app/Contents/MacOS/Chromium"}
	default:
		for _, n := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge"} {
			if p, err := exec.LookPath(n); err == nil {
				cands = append(cands, p)
			}
		}
	}
	for _, c := range cands {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func startCDP(profile string) (*cdpBrowser, error) {
	exe := findChromium()
	if exe == "" {
		return nil, errors.New("nenhum navegador Edge, Chrome ou Chromium encontrado neste computador")
	}
	_ = os.MkdirAll(profile, 0o755)
	_ = os.Remove(filepath.Join(profile, "DevToolsActivePort"))
	cmd := exec.Command(exe, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--window-size=1280,800", "about:blank")
	noConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var port string
	for i := 0; i < 100 && port == ""; i++ {
		if b, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			port = strings.TrimSpace(strings.SplitN(string(b), "\n", 2)[0])
		} else {
			time.Sleep(100 * time.Millisecond)
		}
	}
	if port == "" {
		_ = cmd.Process.Kill()
		return nil, errors.New("o navegador não abriu a porta de depuração")
	}
	var targets []struct{ Type, WebSocketDebuggerURL string }
	for i := 0; i < 30; i++ {
		if resp, err := http.Get("http://127.0.0.1:" + port + "/json/list"); err == nil {
			_ = json.NewDecoder(resp.Body).Decode(&targets)
			resp.Body.Close()
		}
		if len(targets) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	wsURL := ""
	for _, t := range targets {
		if t.Type == "page" {
			wsURL = t.WebSocketDebuggerURL
			break
		}
	}
	if wsURL == "" {
		_ = cmd.Process.Kill()
		return nil, errors.New("o navegador não tem página")
	}
	ws, err := wsDial(wsURL)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	b := &cdpBrowser{profile: profile, cmd: cmd, ws: ws, pending: map[int64]chan json.RawMessage{}, errs: map[int64]string{}, lastUse: time.Now()}
	go b.readLoop()
	for _, m := range []string{"Page.enable", "Runtime.enable", "Log.enable"} {
		_, _ = b.call(context.Background(), m, nil)
	}
	return b, nil
}

func (b *cdpBrowser) readLoop() {
	for {
		raw, err := b.ws.readMessage()
		if err != nil {
			b.mu.Lock()
			b.closed = true
			for id, ch := range b.pending {
				close(ch)
				delete(b.pending, id)
			}
			b.mu.Unlock()
			return
		}
		var m struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.ID != 0 {
			b.mu.Lock()
			ch := b.pending[m.ID]
			delete(b.pending, m.ID)
			if m.Error != nil {
				b.errs[m.ID] = m.Error.Message
			}
			b.mu.Unlock()
			if ch != nil {
				ch <- m.Result
			}
			continue
		}
		switch m.Method {
		case "Runtime.consoleAPICalled":
			var p struct {
				Type string `json:"type"`
				Args []struct {
					Value       any    `json:"value"`
					Description string `json:"description"`
				} `json:"args"`
			}
			_ = json.Unmarshal(m.Params, &p)
			var parts []string
			for _, a := range p.Args {
				if a.Value != nil {
					parts = append(parts, fmt.Sprint(a.Value))
				} else {
					parts = append(parts, a.Description)
				}
			}
			b.addConsole(p.Type + ": " + strings.Join(parts, " "))
		case "Runtime.exceptionThrown":
			var p struct {
				Details struct {
					Text      string `json:"text"`
					Exception struct {
						Description string `json:"description"`
					} `json:"exception"`
				} `json:"exceptionDetails"`
			}
			_ = json.Unmarshal(m.Params, &p)
			b.addConsole("exceção: " + firstNonEmpty(p.Details.Exception.Description, p.Details.Text))
		case "Log.entryAdded":
			var p struct {
				Entry struct{ Level, Text, URL string } `json:"entry"`
			}
			_ = json.Unmarshal(m.Params, &p)
			b.addConsole(p.Entry.Level + ": " + p.Entry.Text + " " + p.Entry.URL)
		}
	}
}

func (b *cdpBrowser) addConsole(s string) {
	b.mu.Lock()
	b.console = append(b.console, truncate(s, 500))
	if len(b.console) > 200 {
		b.console = b.console[len(b.console)-200:]
	}
	b.mu.Unlock()
}

func (b *cdpBrowser) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errors.New("o navegador fechou")
	}
	b.nextID++
	id := b.nextID
	ch := make(chan json.RawMessage, 1)
	b.pending[id] = ch
	b.lastUse = time.Now()
	b.mu.Unlock()
	msg := map[string]any{"id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	if err := b.ws.writeText(raw); err != nil {
		return nil, err
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return nil, errors.New("o navegador fechou")
		}
		b.mu.Lock()
		e := b.errs[id]
		delete(b.errs, id)
		b.mu.Unlock()
		if e != "" {
			return nil, errors.New(e)
		}
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(60 * time.Second):
		return nil, errors.New("o navegador não respondeu em 60 s")
	}
}

// eval runs JavaScript in the page and returns its value as JSON.
func (b *cdpBrowser) eval(ctx context.Context, js string) (json.RawMessage, error) {
	raw, err := b.call(ctx, "Runtime.evaluate", map[string]any{"expression": js, "awaitPromise": true, "returnByValue": true})
	if err != nil {
		return nil, err
	}
	var r struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		Exception *struct {
			Text      string `json:"text"`
			Exception struct {
				Description string `json:"description"`
			} `json:"exception"`
		} `json:"exceptionDetails"`
	}
	_ = json.Unmarshal(raw, &r)
	if r.Exception != nil {
		return nil, errors.New(firstNonEmpty(r.Exception.Exception.Description, r.Exception.Text))
	}
	return r.Result.Value, nil
}

func (b *cdpBrowser) waitLoad(ctx context.Context) {
	for i := 0; i < 100; i++ {
		v, err := b.eval(ctx, "document.readyState")
		if err == nil && string(v) == `"complete"` {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (b *cdpBrowser) close() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_, _ = b.call(ctx, "Browser.close", nil) // lets it end its child processes and release its files
	cancel()
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	if b.ws != nil {
		b.ws.c.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		done := make(chan struct{})
		go func() { _ = b.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			_ = b.cmd.Process.Kill()
			<-done
		}
	}
	// its helper processes outlive the main one for a moment; the profile's lockfile goes when they do
	lock := filepath.Join(b.profile, "lockfile")
	for i := 0; i < 40 && b.profile != ""; i++ {
		if _, err := os.Stat(lock); os.IsNotExist(err) || os.Remove(lock) == nil {
			time.Sleep(150 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

var (
	cdpBrowsers   = map[string]*cdpBrowser{}
	cdpBrowsersMu sync.Mutex
)

// agentBrowser returns the project's headless browser, starting it when needed.
func agentBrowser(root string) (*cdpBrowser, error) {
	cdpBrowsersMu.Lock()
	defer cdpBrowsersMu.Unlock()
	if b := cdpBrowsers[root]; b != nil {
		b.mu.Lock()
		ok := !b.closed
		b.mu.Unlock()
		if ok {
			return b, nil
		}
	}
	sum := sha1.Sum([]byte(filepath.Clean(root)))
	b, err := startCDP(filepath.Join(InfuseHome(), "agent-browser", fmt.Sprintf("%x", sum[:6])))
	if err != nil {
		return nil, err
	}
	cdpBrowsers[root] = b
	go func() { // close after 15 idle minutes
		for {
			time.Sleep(time.Minute)
			b.mu.Lock()
			idle, closed := time.Since(b.lastUse), b.closed
			b.mu.Unlock()
			if closed {
				return
			}
			if idle > 15*time.Minute {
				cdpBrowsersMu.Lock()
				if cdpBrowsers[root] == b {
					delete(cdpBrowsers, root)
				}
				cdpBrowsersMu.Unlock()
				b.close()
				return
			}
		}
	}()
	return b, nil
}

// closeAgentBrowsers is called when the app exits.
func closeAgentBrowsers() {
	cdpBrowsersMu.Lock()
	defer cdpBrowsersMu.Unlock()
	for k, b := range cdpBrowsers {
		b.close()
		delete(cdpBrowsers, k)
	}
}
