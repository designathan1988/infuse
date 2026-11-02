package browser

// The browser pane outside the app window: each tab is a page of a real Chromium-family browser without
// a window, whose screen is streamed to the pane (DevTools screencast) and which receives the user's
// mouse, wheel and keyboard. The HTTP adapter lives in the API; this file owns the pages and the stream.

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"path/filepath"
	"sync"
	"time"
)

type Tab struct {
	page      *Browser
	mu        sync.Mutex
	subs      map[chan []byte]struct{}
	width     int
	height    int
	dpr       float64
	streaming bool
	url       string
	title     string
	frame     []byte // the last frame event, for a new subscriber
}

var (
	viewBrowsers = map[string]*Browser{} // project → browser
	viewTabs     = map[string]*Tab{}     // project|tab → page
	agentTabs    = map[string]*Tab{}     // project|tab → a viewer attached to the agent's own page
	viewMu       sync.Mutex
)

func viewBrowserFor(root string) (*Browser, error) {
	if b := viewBrowsers[root]; b != nil {
		b.mu.Lock()
		ok := !b.closed
		b.mu.Unlock()
		if ok {
			return b, nil
		}
	}
	sum := sha1.Sum([]byte(filepath.Clean(root)))
	b, err := startCDP(filepath.Join(core.InfuseHome(), "view-browser", fmt.Sprintf("%x", sum[:6])))
	if err != nil {
		return nil, err
	}
	viewBrowsers[root] = b
	return b, nil
}

// ViewTab returns the tab's page, creating the browser and the page when needed. A tab attached to
// the agent page is returned as it is: the pane shows the page the agent drives.
func ViewTab(root, id string, w, h int) (*Tab, error) {
	viewMu.Lock()
	defer viewMu.Unlock()
	key := root + "|" + id
	for _, tabs := range []map[string]*Tab{agentTabs, viewTabs} {
		if t := tabs[key]; t != nil {
			t.page.mu.Lock()
			ok := !t.page.closed
			t.page.mu.Unlock()
			if ok {
				return t, nil
			}
		}
	}
	b, err := viewBrowserFor(root)
	if err != nil {
		return nil, err
	}
	page, err := b.newPage("about:blank")
	if err != nil {
		return nil, err
	}
	t := &Tab{page: page, subs: map[chan []byte]struct{}{}}
	page.mu.Lock()
	page.onEvent = t.event
	page.mu.Unlock()
	t.Resize(w, h)
	viewTabs[key] = t
	return t, nil
}

func (t *Tab) broadcast(v any) {
	b, _ := json.Marshal(v)
	t.mu.Lock()
	defer t.mu.Unlock()
	for ch := range t.subs {
		select {
		case ch <- b:
		default: // a slow viewer skips frames rather than holding the browser
		}
	}
}

func (t *Tab) event(method string, params json.RawMessage) {
	switch method {
	case "Page.screencastFrame":
		var f struct {
			Data      string `json:"data"`
			SessionID int    `json:"sessionId"`
			Metadata  struct {
				DeviceWidth  float64 `json:"deviceWidth"`
				DeviceHeight float64 `json:"deviceHeight"`
			} `json:"metadata"`
		}
		_ = json.Unmarshal(params, &f)
		go func() {
			_, _ = t.page.call(context.Background(), "Page.screencastFrameAck", map[string]any{"sessionId": f.SessionID})
		}()
		msg := map[string]any{"type": "frame", "data": f.Data, "w": f.Metadata.DeviceWidth, "h": f.Metadata.DeviceHeight}
		b, _ := json.Marshal(msg)
		t.mu.Lock()
		t.frame = b
		t.mu.Unlock()
		t.broadcast(msg)
	case "Page.frameNavigated":
		var p struct {
			Frame struct {
				ParentID string `json:"parentId"`
				URL      string `json:"url"`
			} `json:"frame"`
		}
		_ = json.Unmarshal(params, &p)
		if p.Frame.ParentID == "" {
			t.mu.Lock()
			t.url = p.Frame.URL
			t.mu.Unlock()
			t.broadcast(map[string]any{"type": "nav", "url": p.Frame.URL, "loading": true})
		}
	case "Page.loadEventFired", "Page.domContentEventFired":
		go func() {
			v, _ := t.page.eval(context.Background(), "document.title")
			var title string
			_ = json.Unmarshal(v, &title)
			t.mu.Lock()
			t.title = title
			url := t.url
			t.mu.Unlock()
			t.broadcast(map[string]any{"type": "nav", "url": url, "title": title, "loading": method != "Page.loadEventFired"})
		}()
	}
}

// StartStream begins the screencast without changing the page's viewport: an attached agent page is
// measured at the size the agent set, never reflowed by the pane.
func (t *Tab) StartStream(w, h int) {
	if w < 200 {
		w = 200
	}
	if h < 150 {
		h = 150
	}
	t.mu.Lock()
	t.width, t.height = w, h
	started := t.streaming
	t.streaming = true
	t.mu.Unlock()
	if !started {
		_, _ = t.page.call(context.Background(), "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 82, "maxWidth": 5120, "maxHeight": 2880, "everyNthFrame": 1})
	}
}

// Resize reflows the page at the pane's size; the screencast runs once with room for any screen.
func (t *Tab) Resize(w, h int, dprs ...float64) {
	dpr := 1.0
	if len(dprs) > 0 && dprs[0] >= 1 && dprs[0] <= 4 {
		dpr = dprs[0]
	}
	if w < 200 {
		w = 200
	}
	if h < 150 {
		h = 150
	}
	t.mu.Lock()
	same := t.width == w && t.height == h && t.dpr == dpr
	t.width, t.height, t.dpr = w, h, dpr
	t.mu.Unlock()
	if same {
		return
	}
	ctx := context.Background()
	_, _ = t.page.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": w, "height": h, "deviceScaleFactor": dpr, "mobile": false})
	t.mu.Lock()
	started := t.streaming
	t.streaming = true
	t.mu.Unlock()
	if !started {
		_, _ = t.page.call(ctx, "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 82, "maxWidth": 5120, "maxHeight": 2880, "everyNthFrame": 1})
	}
}

// Navigate sends the page to url; navErr carries the browser's own reason when it refuses.
func (t *Tab) Navigate(ctx context.Context, url string) (navErr string, err error) {
	raw, err := t.page.call(ctx, "Page.navigate", map[string]any{"url": url})
	var nav struct {
		ErrorText string `json:"errorText"`
	}
	_ = json.Unmarshal(raw, &nav)
	return nav.ErrorText, err
}

// Attach subscribes to the stream: the last frame, the current address and the live channel. The
// returned detach function must be called when the reader stops.
func (t *Tab) Attach() (frame []byte, url, title string, ch chan []byte, detach func()) {
	ch = make(chan []byte, 4)
	t.mu.Lock()
	t.subs[ch] = struct{}{}
	frame, url, title = t.frame, t.url, t.title
	t.mu.Unlock()
	return frame, url, title, ch, func() {
		t.mu.Lock()
		delete(t.subs, ch)
		t.mu.Unlock()
	}
}

// Input applies one pane input event; the pick kind returns the element the user chose.
func (t *Tab) Input(ctx context.Context, kind string, body map[string]any) (json.RawMessage, error) {
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) float64 { f, _ := body[k].(float64); return f }
	switch kind {
	case "resize":
		t.Resize(int(num("width")), int(num("height")), num("dpr"))
		return nil, nil
	case "mouse":
		p := map[string]any{"type": str("type"), "x": num("x"), "y": num("y"), "button": core.FirstNonEmpty(str("button"), "left"), "clickCount": int(num("clickCount")), "modifiers": int(num("modifiers"))}
		if str("type") == "mouseMoved" {
			p["button"] = "none"
		}
		_, err := t.page.call(ctx, "Input.dispatchMouseEvent", p)
		return nil, err
	case "wheel":
		_, err := t.page.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": num("x"), "y": num("y"), "deltaX": num("dx"), "deltaY": num("dy")})
		return nil, err
	case "key":
		p := map[string]any{"type": str("type"), "key": str("key"), "code": str("code"), "windowsVirtualKeyCode": int(num("keyCode")), "nativeVirtualKeyCode": int(num("keyCode")), "modifiers": int(num("modifiers"))}
		if txt := str("text"); txt != "" && str("type") == "keyDown" {
			p["text"] = txt
			p["unmodifiedText"] = txt
		}
		_, err := t.page.call(ctx, "Input.dispatchKeyEvent", p)
		return nil, err
	case "text":
		_, err := t.page.call(ctx, "Input.insertText", map[string]any{"text": str("text")})
		return nil, err
	case "pick":
		v, err := t.page.eval(ctx, fmt.Sprintf(pickAtJS, num("x"), num("y")))
		if err != nil || string(v) == "null" {
			return nil, err
		}
		return json.RawMessage(v), nil
	case "back":
		_, err := t.page.eval(ctx, "history.back()")
		return nil, err
	case "forward":
		_, err := t.page.eval(ctx, "history.forward()")
		return nil, err
	case "reload":
		_, err := t.page.call(ctx, "Page.reload", nil)
		return nil, err
	case "close":
		t.Close()
		return nil, nil
	}
	return nil, nil
}

func (t *Tab) Close() {
	viewMu.Lock()
	for _, tabs := range []map[string]*Tab{viewTabs, agentTabs} {
		for k, v := range tabs {
			if v == t {
				delete(tabs, k)
			}
		}
	}
	viewMu.Unlock()
	t.page.close()
}

// ViewAgentTab attaches the pane to the page the agent's Browser tool drives: a second CDP
// connection on the same target, streamed like any other tab. The agent keeps controlling the page;
// the pane sees its screens and its address without resizing it.
func ViewAgentTab(root, id string, st config.Settings, w, h int) (*Tab, error) {
	viewMu.Lock()
	defer viewMu.Unlock()
	key := root + "|" + id
	if t := agentTabs[key]; t != nil {
		t.page.mu.Lock()
		ok := !t.page.closed
		t.page.mu.Unlock()
		if ok {
			t.StartStream(w, h)
			return t, nil
		}
	}
	p, err := AgentPageFor(root, st)
	if err != nil {
		return nil, err
	}
	p.b.mu.Lock()
	port, target := p.b.port, p.b.targetID
	p.b.mu.Unlock()
	if port == "" || target == "" {
		return nil, fmt.Errorf("the agent page has no debuggable target")
	}
	ws, err := wsDial("ws://127.0.0.1:" + port + "/devtools/page/" + target)
	if err != nil {
		return nil, err
	}
	b := &Browser{port: port, targetID: target, ws: ws, detached: true, pending: map[int64]chan json.RawMessage{}, errs: map[int64]string{}, lastUse: time.Now()}
	t := &Tab{page: b, subs: map[chan []byte]struct{}{}}
	b.onEvent = t.event
	go b.readLoop()
	// events and the screencast belong to this connection: Page.enable is required on it, even
	// though the agent's own connection enabled the domain earlier.
	_, _ = b.call(context.Background(), "Page.enable", nil)
	_, _ = b.call(context.Background(), "Runtime.enable", nil)
	// the page is already where the agent left it: read the address now, so the pane opens on it
	var url, title string
	if raw, err := b.eval(context.Background(), "location.href"); err == nil {
		_ = json.Unmarshal(raw, &url)
	}
	if raw, err := b.eval(context.Background(), "document.title"); err == nil {
		_ = json.Unmarshal(raw, &title)
	}
	t.mu.Lock()
	t.url, t.title = url, title
	t.mu.Unlock()
	t.StartStream(w, h)
	agentTabs[key] = t
	return t, nil
}

// CloseViewTabs closes every streamed browser (application shutdown).
func CloseViewTabs() {
	viewMu.Lock()
	defer viewMu.Unlock()
	for k, b := range viewBrowsers {
		b.close()
		delete(viewBrowsers, k)
	}
	for k, t := range agentTabs {
		t.page.close() // detached: closes the viewer's connection, never the agent's page
		delete(agentTabs, k)
	}
	viewTabs = map[string]*Tab{}
}

// pickAtJS describes the element at (x, y): selector, HTML, text, box and main styles.
const pickAtJS = `(() => { const el = document.elementFromPoint(%v, %v); if (!el) return null;
  const sel = el => { if (el.id) return '#' + CSS.escape(el.id); const p = []; for (let e = el; e && e.nodeType === 1 && e !== document.documentElement; e = e.parentElement) { let s = e.tagName.toLowerCase(); if (e.id) { p.unshift('#' + CSS.escape(e.id)); break; } const cls = [...e.classList].slice(0, 2).map(c => '.' + CSS.escape(c)).join(''); const sib = e.parentElement ? [...e.parentElement.children].filter(x => x.tagName === e.tagName) : []; s += cls + (sib.length > 1 ? ':nth-of-type(' + (sib.indexOf(e) + 1) + ')' : ''); p.unshift(s); } return p.join(' > '); };
  const r = el.getBoundingClientRect(), cs = getComputedStyle(el);
  return { url: location.href, title: document.title, selector: sel(el), html: el.outerHTML.slice(0, 4000), text: (el.innerText || '').trim().slice(0, 500), rect: { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) }, style: { color: cs.color, background: cs.backgroundColor, font: cs.fontSize + ' ' + cs.fontFamily.split(',')[0], margin: cs.margin, padding: cs.padding } }; })()`
