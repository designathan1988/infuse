package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// A headless Chromium-family browser: discovery, launch, pages and RPC.

type Browser struct {
	port     string
	parent   *Browser // set on extra pages (tabs) of a browser
	targetID string
	onEvent  func(method string, params json.RawMessage) // events other than console (screencast, navigation)
	profile  string
	cmd      *exec.Cmd
	ws       *wsConn
	mu       sync.Mutex
	nextID   int64
	pending  map[int64]chan json.RawMessage
	errs     map[int64]string
	console  []string
	lastUse  time.Time
	closed   bool
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

func startCDP(profile string) (*Browser, error) {
	exe := findChromium()
	if exe == "" {
		return nil, errors.New("nenhum navegador Edge, Chrome ou Chromium encontrado neste computador")
	}
	_ = os.MkdirAll(profile, 0o755)
	_ = os.Remove(filepath.Join(profile, "DevToolsActivePort"))
	cmd := exec.Command(exe, "--headless=new", "--remote-debugging-port=0", "--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-blink-features=AutomationControlled", "--window-size=1280,800", "about:blank")
	process.NoConsoleWindow(cmd)
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
		return nil, errors.New("the browser did not open the debugging port")
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
		return nil, errors.New("the browser has no page")
	}
	ws, err := wsDial(wsURL)
	if err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	b := &Browser{port: port, profile: profile, cmd: cmd, ws: ws, pending: map[int64]chan json.RawMessage{}, errs: map[int64]string{}, lastUse: time.Now()}
	go b.readLoop()
	b.prepare()
	return b, nil
}

// prepare enables the domains infuse listens to and presents the browser as a regular one (sites block
// or degrade "HeadlessChrome").
func (b *Browser) prepare() {
	for _, m := range []string{"Page.enable", "Runtime.enable", "Log.enable"} {
		_, _ = b.call(context.Background(), m, nil)
	}
	if raw, err := b.call(context.Background(), "Browser.getVersion", nil); err == nil {
		var v struct {
			UserAgent string `json:"userAgent"`
		}
		_ = json.Unmarshal(raw, &v)
		if ua := strings.Replace(v.UserAgent, "HeadlessChrome", "Chrome", 1); ua != "" {
			_, _ = b.call(context.Background(), "Network.setUserAgentOverride", map[string]any{"userAgent": ua, "acceptLanguage": "pt-BR,pt;q=0.9,en;q=0.8"})
		}
	}
}

// newPage opens another page (a tab) in the same browser, with its own connection.
func (b *Browser) newPage(url string) (*Browser, error) {
	raw, err := b.call(context.Background(), "Target.createTarget", map[string]any{"url": url})
	if err != nil {
		return nil, err
	}
	var t struct {
		TargetID string `json:"targetId"`
	}
	_ = json.Unmarshal(raw, &t)
	ws, err := wsDial("ws://127.0.0.1:" + b.port + "/devtools/page/" + t.TargetID)
	if err != nil {
		return nil, err
	}
	p := &Browser{port: b.port, parent: b, targetID: t.TargetID, ws: ws, pending: map[int64]chan json.RawMessage{}, errs: map[int64]string{}, lastUse: time.Now()}
	go p.readLoop()
	p.prepare()
	return p, nil
}

// cdpMessage is one DevTools protocol message, a response or an event.
type cdpMessage struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (b *Browser) readLoop() {
	for {
		raw, err := b.ws.readMessage()
		if err != nil {
			b.failPending()
			return
		}
		var m cdpMessage
		if json.Unmarshal(raw, &m) != nil {
			continue
		}
		if m.ID != 0 {
			b.deliverResponse(m)
			continue
		}
		b.mu.Lock()
		hook := b.onEvent
		b.mu.Unlock()
		if hook != nil {
			hook(m.Method, m.Params)
		}
		b.logConsoleEvent(m)
	}
}

// failPending marks the browser closed and wakes everyone waiting on a response.
func (b *Browser) failPending() {
	b.mu.Lock()
	b.closed = true
	for id, ch := range b.pending {
		close(ch)
		delete(b.pending, id)
	}
	b.mu.Unlock()
}

// deliverResponse hands a command's result (or error) to its caller.
func (b *Browser) deliverResponse(m cdpMessage) {
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
}

// logConsoleEvent keeps console messages, exceptions and log entries for the console tool.
func (b *Browser) logConsoleEvent(m cdpMessage) {
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
		b.addConsole("exception: " + core.FirstNonEmpty(p.Details.Exception.Description, p.Details.Text))
	case "Log.entryAdded":
		var p struct {
			Entry struct{ Level, Text, URL string } `json:"entry"`
		}
		_ = json.Unmarshal(m.Params, &p)
		b.addConsole(p.Entry.Level + ": " + p.Entry.Text + " " + p.Entry.URL)
	}
}

func (b *Browser) addConsole(s string) {
	b.mu.Lock()
	b.console = append(b.console, core.Truncate(s, 500))
	if len(b.console) > 200 {
		b.console = b.console[len(b.console)-200:]
	}
	b.mu.Unlock()
}

func (b *Browser) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
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
		return nil, errors.New("the browser did not respond within 60 s")
	}
}

// eval runs JavaScript in the page and returns its value as JSON.
// Call sends one DevTools command (the docs fetcher uses it).
func (b *Browser) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	return b.call(ctx, method, params)
}

// Eval evaluates JavaScript in the page.
func (b *Browser) Eval(ctx context.Context, js string) (json.RawMessage, error) {
	return b.eval(ctx, js)
}

func (b *Browser) eval(ctx context.Context, js string) (json.RawMessage, error) {
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
		return nil, errors.New(core.FirstNonEmpty(r.Exception.Exception.Description, r.Exception.Text))
	}
	return r.Result.Value, nil
}

// WaitLoad waits for the page load event.
func (b *Browser) WaitLoad(ctx context.Context) { b.waitLoad(ctx) }

func (b *Browser) waitLoad(ctx context.Context) {
	for i := 0; i < 100; i++ {
		v, err := b.eval(ctx, "document.readyState")
		if err == nil && string(v) == `"complete"` {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
}

func (b *Browser) close() {
	if b.parent != nil { // a tab: close its target, the browser stays
		_, _ = b.parent.call(context.Background(), "Target.closeTarget", map[string]any{"targetId": b.targetID})
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		b.ws.c.Close()
		return
	}
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
