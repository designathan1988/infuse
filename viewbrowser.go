package main

// The browser pane outside the app window (the UI in a regular browser, where an iframe is refused by
// most sites): each tab is a page of a real Chromium-family browser without a window, whose screen is
// streamed to the pane (DevTools screencast) and which receives the user's mouse, wheel and keyboard.
// Any site works, at full fidelity. Inside the app window the pane uses native WebView2 views instead.

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"
)

type viewTab struct {
	page      *cdpBrowser
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
	viewBrowsers = map[string]*cdpBrowser{} // project → browser
	viewTabs     = map[string]*viewTab{}    // project|tab → page
	viewMu       sync.Mutex
)

func viewBrowserFor(root string) (*cdpBrowser, error) {
	if b := viewBrowsers[root]; b != nil {
		b.mu.Lock()
		ok := !b.closed
		b.mu.Unlock()
		if ok {
			return b, nil
		}
	}
	sum := sha1.Sum([]byte(filepath.Clean(root)))
	b, err := startCDP(filepath.Join(InfuseHome(), "view-browser", fmt.Sprintf("%x", sum[:6])))
	if err != nil {
		return nil, err
	}
	viewBrowsers[root] = b
	return b, nil
}

// viewTabFor returns the tab's page, creating the browser and the page when needed.
func viewTabFor(root, id string, w, h int) (*viewTab, error) {
	viewMu.Lock()
	defer viewMu.Unlock()
	key := root + "|" + id
	if t := viewTabs[key]; t != nil {
		t.page.mu.Lock()
		ok := !t.page.closed
		t.page.mu.Unlock()
		if ok {
			return t, nil
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
	t := &viewTab{page: page, subs: map[chan []byte]struct{}{}}
	page.mu.Lock()
	page.onEvent = t.event
	page.mu.Unlock()
	t.resize(w, h)
	viewTabs[key] = t
	return t, nil
}

func (t *viewTab) broadcast(v any) {
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

func (t *viewTab) event(method string, params json.RawMessage) {
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

func (t *viewTab) resize(w, h int, dprs ...float64) {
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
	// drawn at the screen's real density, shown at its CSS size: sharp and never stretched. The page
	// reflows at once; the frame stream, started once with room for any screen, keeps running.
	_, _ = t.page.call(ctx, "Emulation.setDeviceMetricsOverride", map[string]any{"width": w, "height": h, "deviceScaleFactor": dpr, "mobile": false})
	t.mu.Lock()
	started := t.streaming
	t.streaming = true
	t.mu.Unlock()
	if !started {
		_, _ = t.page.call(ctx, "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 82, "maxWidth": 5120, "maxHeight": 2880, "everyNthFrame": 1})
	}
}

// ---------------------------------------------------------------- HTTP

func viewAPI(w http.ResponseWriter, r *http.Request, root, path string, body map[string]any) bool {
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) float64 { f, _ := body[k].(float64); return f }
	id := str("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	switch path {
	case "/api/view/stream":
		t, err := viewTabFor(root, id, 1024, 700)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return true
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			return true
		}
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("cache-control", "no-cache")
		ch := make(chan []byte, 4)
		t.mu.Lock()
		t.subs[ch] = struct{}{}
		last, url, title := t.frame, t.url, t.title
		t.mu.Unlock()
		defer func() { t.mu.Lock(); delete(t.subs, ch); t.mu.Unlock() }()
		nav, _ := json.Marshal(map[string]any{"type": "nav", "url": url, "title": title})
		fmt.Fprintf(w, "data: %s\n\n", nav)
		if last != nil {
			fmt.Fprintf(w, "data: %s\n\n", last)
		}
		fl.Flush()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case b := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			case <-r.Context().Done():
				return true
			}
		}
	case "/api/view/open":
		t, err := viewTabFor(root, id, int(num("width")), int(num("height")))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		t.resize(int(num("width")), int(num("height")), num("dpr"))
		if u := str("url"); u != "" {
			raw, err := t.page.call(r.Context(), "Page.navigate", map[string]any{"url": u})
			var nav struct {
				ErrorText string `json:"errorText"`
			}
			_ = json.Unmarshal(raw, &nav)
			if err != nil || nav.ErrorText != "" {
				sendJSON(w, 200, map[string]any{"ok": false, "error": firstNonEmpty(nav.ErrorText, errString(err))})
				return true
			}
		}
		sendJSON(w, 200, map[string]any{"ok": true})
	case "/api/view/input":
		t, err := viewTabFor(root, id, 0, 0)
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		ctx := r.Context()
		switch str("kind") {
		case "resize":
			t.resize(int(num("width")), int(num("height")), num("dpr"))
		case "mouse": // mousePressed | mouseReleased | mouseMoved
			p := map[string]any{"type": str("type"), "x": num("x"), "y": num("y"), "button": firstNonEmpty(str("button"), "left"), "clickCount": int(num("clickCount")), "modifiers": int(num("modifiers"))}
			if str("type") == "mouseMoved" {
				p["button"] = "none"
			}
			_, err = t.page.call(ctx, "Input.dispatchMouseEvent", p)
		case "wheel":
			_, err = t.page.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": num("x"), "y": num("y"), "deltaX": num("dx"), "deltaY": num("dy")})
		case "key": // keyDown/keyUp with the browser's own key description
			p := map[string]any{"type": str("type"), "key": str("key"), "code": str("code"), "windowsVirtualKeyCode": int(num("keyCode")), "nativeVirtualKeyCode": int(num("keyCode")), "modifiers": int(num("modifiers"))}
			if t := str("text"); t != "" && str("type") == "keyDown" {
				p["text"] = t
				p["unmodifiedText"] = t
			}
			_, err = t.page.call(ctx, "Input.dispatchKeyEvent", p)
		case "text":
			_, err = t.page.call(ctx, "Input.insertText", map[string]any{"text": str("text")})
		case "pick": // the element at the point the user clicked, for the message
			v, e := t.page.eval(ctx, fmt.Sprintf(pickAtJS, num("x"), num("y")))
			if e != nil || string(v) == "null" {
				sendJSON(w, 200, map[string]any{"ok": false, "error": firstNonEmpty(errString(e), "nada nesse ponto")})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "pick": json.RawMessage(v)})
			return true
		case "back":
			_, err = t.page.eval(ctx, "history.back()")
		case "forward":
			_, err = t.page.eval(ctx, "history.forward()")
		case "reload":
			_, err = t.page.call(ctx, "Page.reload", nil)
		case "close":
			viewMu.Lock()
			delete(viewTabs, root+"|"+id)
			viewMu.Unlock()
			t.page.close()
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
	default:
		return false
	}
	return true
}

// closeViewBrowsers is called when the app exits.
func closeViewBrowsers() {
	viewMu.Lock()
	defer viewMu.Unlock()
	for k, b := range viewBrowsers {
		b.close()
		delete(viewBrowsers, k)
	}
	viewTabs = map[string]*viewTab{}
}

// pickAtJS describes the element at (x, y): selector, HTML, text, box and main styles.
const pickAtJS = `(() => { const el = document.elementFromPoint(%v, %v); if (!el) return null;
  const sel = el => { if (el.id) return '#' + CSS.escape(el.id); const p = []; for (let e = el; e && e.nodeType === 1 && e !== document.documentElement; e = e.parentElement) { let s = e.tagName.toLowerCase(); if (e.id) { p.unshift('#' + CSS.escape(e.id)); break; } const cls = [...e.classList].slice(0, 2).map(c => '.' + CSS.escape(c)).join(''); const sib = e.parentElement ? [...e.parentElement.children].filter(x => x.tagName === e.tagName) : []; s += cls + (sib.length > 1 ? ':nth-of-type(' + (sib.indexOf(e) + 1) + ')' : ''); p.unshift(s); } return p.join(' > '); };
  const r = el.getBoundingClientRect(), cs = getComputedStyle(el);
  return { url: location.href, title: document.title, selector: sel(el), html: el.outerHTML.slice(0, 4000), text: (el.innerText || '').trim().slice(0, 500), rect: { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) }, style: { color: cs.color, background: cs.backgroundColor, font: cs.fontSize + ' ' + cs.fontFamily.split(',')[0], margin: cs.margin, padding: cs.padding } }; })()`
