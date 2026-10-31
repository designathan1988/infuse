package browser

// The agent's browser, made to look and act like a person using it: a visible cursor that travels along
// curved, accelerating paths, clicks with a pause and a ripple, typing key by key at a varying rhythm,
// scrolling in steps; screenshots at the screen density the user sets; recordings handed to the model as
// the keyframes where the screen changed, with a timeline of actions, console and network between them
// (a model reads still frames, not video); and the diagnostics a developer opens DevTools for — network,
// console, performance, accessibility tree, storage, device and network emulation. Locators follow
// Playwright's: role=, text=, label=, placeholder=, testid=, xpath= or CSS.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type netEntry struct {
	ID       string  `json:"-"`
	Method   string  `json:"method"`
	URL      string  `json:"url"`
	Type     string  `json:"type"`
	Status   int     `json:"status"`
	Mime     string  `json:"mime,omitempty"`
	Bytes    float64 `json:"bytes"`
	Ms       int64   `json:"ms"`
	Failed   string  `json:"failed,omitempty"`
	start    time.Time
	finished bool
}

type cursorPoint struct {
	at   time.Duration
	x, y float64
}

type recFrame struct {
	at   time.Duration
	jpeg []byte
}

// Page is the state infuse keeps about the agent's page.
type Page struct {
	b        *Browser
	mu       sync.Mutex
	x, y     float64 // the cursor
	net      []*netEntry
	netByID  map[string]*netEntry
	rec      bool
	recStart time.Time
	frames   []recFrame
	events   []recEvent
	trail    []cursorPoint // where the agent's cursor was during the recording (left out of change detection)
	width    int
	height   int
	scale    float64
	device   string
}

var (
	Pages   = map[*Browser]*Page{}
	PagesMu sync.Mutex
)

// cursorJS draws the agent's cursor on every page (kept across navigations) and the click ripple.
const cursorJS = `(() => { if (window.__infuseCursor) return;
  const mk = () => { const c = document.createElement('div'); c.id = '__infuse_cursor';
    c.style.cssText = 'position:fixed;left:-40px;top:-40px;width:24px;height:24px;z-index:2147483647;pointer-events:none;filter:drop-shadow(0 1px 2px rgba(0,0,0,.45))';
    c.innerHTML = '<svg width="24" height="24" viewBox="0 0 24 24"><path d="M3 2 L3 19 L7.6 14.6 L10.8 21.6 L13.7 20.3 L10.6 13.5 L17 13.5 Z" fill="#fff" stroke="#111" stroke-width="1.5" stroke-linejoin="round"/></svg>';
    (document.body || document.documentElement).appendChild(c); return c; };
  window.__infuseCursor = {
    move(x, y) { const c = document.getElementById('__infuse_cursor') || mk(); c.style.left = (x - 3) + 'px'; c.style.top = (y - 2) + 'px'; },
    click(x, y) { const r = document.createElement('div');
      r.style.cssText = 'position:fixed;left:' + (x - 15) + 'px;top:' + (y - 15) + 'px;width:30px;height:30px;border-radius:50%;border:2.5px solid rgba(76,111,214,.95);background:rgba(76,111,214,.18);z-index:2147483646;pointer-events:none;transition:transform .5s ease-out,opacity .5s ease-out';
      document.documentElement.appendChild(r); requestAnimationFrame(() => { r.style.transform = 'scale(1.9)'; r.style.opacity = '0'; }); setTimeout(() => r.remove(), 600); } }; })()`

// agentPageFor sets up the agent's page once: cursor, network log, viewport and density from settings.
func AgentPageFor(root string, st config.Settings) (*Page, error) {
	b, err := AgentBrowser(root)
	if err != nil {
		return nil, err
	}
	PagesMu.Lock()
	p := Pages[b]
	if p == nil {
		p = &Page{b: b, netByID: map[string]*netEntry{}, x: -1, y: -1}
		Pages[b] = p
	}
	PagesMu.Unlock()
	ctx := context.Background()
	b.mu.Lock()
	fresh := b.onEvent == nil
	if fresh {
		b.onEvent = p.event
	}
	b.mu.Unlock()
	if fresh {
		_, _ = b.call(ctx, "Network.enable", nil)
		_, _ = b.call(ctx, "Performance.enable", nil) // counters from the first page on
		_, _ = b.call(ctx, "Page.addScriptToEvaluateOnNewDocument", map[string]any{"source": cursorJS})
		_, _ = b.eval(ctx, cursorJS)
	}
	w, h, scale := browserViewport(st)
	p.mu.Lock()
	change := p.device == "" && (p.width != w || p.height != h || p.scale != scale)
	p.mu.Unlock()
	if change {
		p.setMetrics(w, h, scale, false)
	}
	return p, nil
}

// browserViewport is the agent browser's window from settings (default 1440×900 at 2×: sharp captures).
func browserViewport(st config.Settings) (int, int, float64) {
	w, h := 1440, 900
	if st.BrowserViewport != "" {
		_, _ = fmt.Sscanf(strings.ReplaceAll(st.BrowserViewport, "×", "x"), "%dx%d", &w, &h)
	}
	scale := st.BrowserScale
	if scale <= 0 {
		scale = 2
	}
	return max(320, w), max(240, h), math.Min(scale, 4)
}

// Eval evaluates JavaScript in the page (tests and diagnostics).
func (p *Page) Eval(ctx context.Context, js string) (json.RawMessage, error) { return p.b.eval(ctx, js) }

func (p *Page) setMetrics(w, h int, scale float64, mobile bool) {
	_, _ = p.b.call(context.Background(), "Emulation.setDeviceMetricsOverride", map[string]any{"width": w, "height": h, "deviceScaleFactor": scale, "mobile": mobile})
	p.mu.Lock()
	p.width, p.height, p.scale = w, h, scale
	p.mu.Unlock()
}

func (p *Page) event(method string, params json.RawMessage) {
	switch method {
	case "Network.requestWillBeSent":
		var e struct {
			RequestID string                       `json:"requestId"`
			Type      string                       `json:"type"`
			Request   struct{ Method, URL string } `json:"request"`
		}
		_ = json.Unmarshal(params, &e)
		n := &netEntry{ID: e.RequestID, Method: e.Request.Method, URL: e.Request.URL, Type: e.Type, start: time.Now()}
		p.mu.Lock()
		p.net = append(p.net, n)
		if len(p.net) > 400 {
			delete(p.netByID, p.net[0].ID)
			p.net = p.net[1:]
		}
		p.netByID[e.RequestID] = n
		p.mu.Unlock()
	case "Network.responseReceived":
		var e struct {
			RequestID string `json:"requestId"`
			Response  struct {
				Status   int    `json:"status"`
				MimeType string `json:"mimeType"`
			} `json:"response"`
		}
		_ = json.Unmarshal(params, &e)
		p.mu.Lock()
		if n := p.netByID[e.RequestID]; n != nil {
			n.Status, n.Mime = e.Response.Status, e.Response.MimeType
		}
		p.mu.Unlock()
	case "Network.loadingFinished", "Network.loadingFailed":
		var e struct {
			RequestID         string  `json:"requestId"`
			EncodedDataLength float64 `json:"encodedDataLength"`
			ErrorText         string  `json:"errorText"`
		}
		_ = json.Unmarshal(params, &e)
		p.mu.Lock()
		n := p.netByID[e.RequestID]
		if n != nil {
			n.Bytes, n.Failed, n.finished = e.EncodedDataLength, e.ErrorText, true
			n.Ms = time.Since(n.start).Milliseconds()
		}
		p.mu.Unlock()
		if n != nil && (n.Failed != "" || n.Status >= 400) {
			p.note(core.M("browser.ev_network", "method", n.Method, "url", core.Truncate(n.URL, 120), "result", core.FirstNonEmpty(n.Failed, fmt.Sprint(n.Status))))
		}
	case "Page.screencastFrame":
		var f struct {
			Data      string `json:"data"`
			SessionID int    `json:"sessionId"`
		}
		_ = json.Unmarshal(params, &f)
		go func() {
			_, _ = p.b.call(context.Background(), "Page.screencastFrameAck", map[string]any{"sessionId": f.SessionID})
		}()
		if j, err := base64.StdEncoding.DecodeString(f.Data); err == nil {
			p.mu.Lock()
			if p.rec && len(p.frames) < 3000 {
				p.frames = append(p.frames, recFrame{time.Since(p.recStart), j})
			}
			p.mu.Unlock()
		}
	case "Runtime.exceptionThrown":
		var e struct {
			Details struct {
				Text      string `json:"text"`
				Exception struct {
					Description string `json:"description"`
				} `json:"exception"`
			} `json:"exceptionDetails"`
		}
		_ = json.Unmarshal(params, &e)
		p.note(core.M("browser.ev_exception", "text", core.Truncate(core.FirstLine(core.FirstNonEmpty(e.Details.Exception.Description, e.Details.Text)), 200)))
	case "Runtime.consoleAPICalled":
		var e struct {
			Type string `json:"type"`
			Args []struct {
				Value       any    `json:"value"`
				Description string `json:"description"`
			} `json:"args"`
		}
		_ = json.Unmarshal(params, &e)
		if e.Type == "error" || e.Type == "warning" {
			var parts []string
			for _, a := range e.Args {
				parts = append(parts, core.FirstNonEmpty(fmt.Sprint(a.Value), a.Description))
			}
			p.note(core.M("browser.ev_console", "type", e.Type, "text", core.Truncate(strings.Join(parts, " "), 200)))
		}
	case "Page.frameNavigated":
		var e struct {
			Frame struct {
				ParentID string `json:"parentId"`
				URL      string `json:"url"`
			} `json:"frame"`
		}
		_ = json.Unmarshal(params, &e)
		if e.Frame.ParentID == "" {
			p.note(core.M("browser.ev_navigated", "url", core.Truncate(e.Frame.URL, 160)))
		}
	case "Page.loadEventFired":
		p.mu.Lock()
		x, y := p.x, p.y
		p.mu.Unlock()
		if x >= 0 {
			go func() {
				_, _ = p.b.eval(context.Background(), fmt.Sprintf("window.__infuseCursor && __infuseCursor.move(%f,%f)", x, y))
			}()
		}
	}
}

// ---------------------------------------------------------------- human gestures

type gestureStyle struct {
	stepPx    float64       // path resolution
	stepDelay time.Duration // between path points
	preClick  time.Duration // hover before pressing
	press     time.Duration // button held
	keyMin    time.Duration // typing rhythm
	keyMax    time.Duration
}

func gesturesFor(st config.Settings) gestureStyle {
	switch st.BrowserGestures {
	case "instant":
		return gestureStyle{stepPx: 1e9}
	case "slow":
		return gestureStyle{stepPx: 10, stepDelay: 16 * time.Millisecond, preClick: 260 * time.Millisecond, press: 140 * time.Millisecond, keyMin: 110 * time.Millisecond, keyMax: 260 * time.Millisecond}
	}
	return gestureStyle{stepPx: 18, stepDelay: 9 * time.Millisecond, preClick: 110 * time.Millisecond, press: 80 * time.Millisecond, keyMin: 45 * time.Millisecond, keyMax: 140 * time.Millisecond}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// moveTo travels from the cursor to (x, y) along a curved path with ease-in-out, like a hand would.
func (p *Page) moveTo(ctx context.Context, g gestureStyle, x, y float64, buttons int) {
	p.mu.Lock()
	x0, y0 := p.x, p.y
	w, h := p.width, p.height
	p.mu.Unlock()
	if x0 < 0 {
		x0, y0 = float64(w)*0.5+rand.Float64()*40, float64(h)*0.9
	}
	d := math.Hypot(x-x0, y-y0)
	steps := int(math.Max(1, math.Min(60, d/g.stepPx)))
	// a control point off the straight line: hands do not move in straight lines
	nx, ny := -(y - y0), x-x0
	if d > 0 {
		nx, ny = nx/d, ny/d
	}
	bend := (rand.Float64() - 0.5) * d * 0.3
	cx, cy := (x0+x)/2+nx*bend, (y0+y)/2+ny*bend
	for i := 1; i <= steps; i++ {
		t := float64(i) / float64(steps)
		t = t * t * (3 - 2*t) // ease in and out
		px := (1-t)*(1-t)*x0 + 2*(1-t)*t*cx + t*t*x
		py := (1-t)*(1-t)*y0 + 2*(1-t)*t*cy + t*t*y
		if i == steps {
			px, py = x, y
		}
		p.mouse(ctx, "mouseMoved", px, py, buttons, 0)
		sleepCtx(ctx, g.stepDelay)
	}
}

func (p *Page) mouse(ctx context.Context, typ string, x, y float64, buttons, clicks int) {
	params := map[string]any{"type": typ, "x": x, "y": y, "buttons": buttons}
	if typ != "mouseMoved" {
		params["button"] = "left"
		params["clickCount"] = clicks
	} else if buttons == 1 {
		params["button"] = "left"
	}
	_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", params)
	p.mu.Lock()
	p.x, p.y = x, y
	if p.rec {
		p.trail = append(p.trail, cursorPoint{time.Since(p.recStart), x, y})
	}
	p.mu.Unlock()
	_, _ = p.b.eval(ctx, fmt.Sprintf("window.__infuseCursor && __infuseCursor.move(%f,%f)", x, y))
}

func (p *Page) click(ctx context.Context, g gestureStyle, x, y float64, button string, clicks int) {
	p.moveTo(ctx, g, x, y, 0)
	sleepCtx(ctx, g.preClick)
	_, _ = p.b.eval(ctx, fmt.Sprintf("window.__infuseCursor && __infuseCursor.click(%f,%f)", x, y))
	for c := 1; c <= clicks; c++ {
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mousePressed", "x": x, "y": y, "button": button, "buttons": 1, "clickCount": c})
		sleepCtx(ctx, g.press)
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseReleased", "x": x, "y": y, "button": button, "buttons": 0, "clickCount": c})
		if c < clicks {
			sleepCtx(ctx, 60*time.Millisecond)
		}
	}
}

// keyInfo describes a key for DevTools: its key, code and Windows key code.
func keyInfo(k string) (key, code string, vk int) {
	named := map[string][2]any{"Enter": {"Enter", 13}, "Tab": {"Tab", 9}, "Escape": {"Escape", 27}, "Backspace": {"Backspace", 8}, "Delete": {"Delete", 46}, "ArrowDown": {"ArrowDown", 40}, "ArrowUp": {"ArrowUp", 38}, "ArrowLeft": {"ArrowLeft", 37}, "ArrowRight": {"ArrowRight", 39}, "Home": {"Home", 36}, "End": {"End", 35}, "PageDown": {"PageDown", 34}, "PageUp": {"PageUp", 33}, "Space": {"Space", 32}, " ": {"Space", 32}, "F5": {"F5", 116}, "F12": {"F12", 123}}
	if v, ok := named[k]; ok {
		if k == "Space" || k == " " {
			return " ", "Space", 32
		}
		return k, v[0].(string), v[1].(int)
	}
	if len(k) == 1 {
		c := k[0]
		switch {
		case c >= 'a' && c <= 'z':
			return k, "Key" + strings.ToUpper(k), int(c - 32)
		case c >= 'A' && c <= 'Z':
			return k, "Key" + k, int(c)
		case c >= '0' && c <= '9':
			return k, "Digit" + k, int(c)
		}
		return k, "", 0
	}
	return k, k, 0
}

// press sends a key or a combination like "Control+A" or "Shift+Tab", as a browser receives it.
func (p *Page) press(ctx context.Context, combo string) error {
	parts := strings.Split(combo, "+")
	if strings.HasSuffix(combo, "++") { // "Control++"
		parts = append(parts[:len(parts)-2], "+")
	}
	mods := 0
	modBits := map[string]int{"alt": 1, "control": 2, "ctrl": 2, "meta": 4, "cmd": 4, "shift": 8}
	var keyName string
	for _, part := range parts {
		if bit, ok := modBits[strings.ToLower(part)]; ok {
			mods |= bit
		} else {
			keyName = part
		}
	}
	if keyName == "" {
		return core.ErrMsg(core.M("browser.no_key", "combo", combo))
	}
	key, code, vk := keyInfo(keyName)
	down := map[string]any{"type": "keyDown", "key": key, "code": code, "windowsVirtualKeyCode": vk, "nativeVirtualKeyCode": vk, "modifiers": mods}
	if mods&(2|4|1) == 0 { // text only without Ctrl/Alt/Meta
		switch {
		case key == "Enter":
			down["text"] = "\r"
		case len([]rune(key)) == 1:
			txt := key
			if mods&8 != 0 {
				txt = strings.ToUpper(txt)
			}
			down["text"] = txt
		}
	}
	if mods&2 != 0 && strings.EqualFold(key, "a") {
		down["commands"] = []string{"selectAll"}
	}
	if _, err := p.b.call(ctx, "Input.dispatchKeyEvent", down); err != nil {
		return err
	}
	_, err := p.b.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": code, "windowsVirtualKeyCode": vk, "nativeVirtualKeyCode": vk, "modifiers": mods})
	return err
}

// typeText types like a person: key by key, at an uneven rhythm (accented characters as text input).
func (p *Page) typeText(ctx context.Context, g gestureStyle, text string) {
	if g.keyMax == 0 {
		_, _ = p.b.call(ctx, "Input.insertText", map[string]any{"text": text})
		return
	}
	for _, r := range text {
		ch := string(r)
		key, code, vk := keyInfo(ch)
		if r == '\n' {
			_ = p.press(ctx, "Enter")
		} else if vk != 0 || r == ' ' || (r < 128 && r > 31) {
			down := map[string]any{"type": "keyDown", "key": key, "code": code, "windowsVirtualKeyCode": vk, "text": ch, "unmodifiedText": ch}
			_, _ = p.b.call(ctx, "Input.dispatchKeyEvent", down)
			_, _ = p.b.call(ctx, "Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": key, "code": code, "windowsVirtualKeyCode": vk})
		} else {
			_, _ = p.b.call(ctx, "Input.insertText", map[string]any{"text": ch})
		}
		d := g.keyMin + time.Duration(rand.Int63n(int64(g.keyMax-g.keyMin)+1))
		if r == ' ' || r == '.' || r == ',' {
			d += g.keyMin // people pause between words
		}
		sleepCtx(ctx, d)
	}
}

// scroll turns the wheel in steps at the cursor (or the page centre).
func (p *Page) scroll(ctx context.Context, g gestureStyle, dy float64) {
	p.mu.Lock()
	x, y := p.x, p.y
	if x < 0 {
		x, y = float64(p.width)/2, float64(p.height)/2
	}
	p.mu.Unlock()
	steps := int(math.Max(1, math.Abs(dy)/120))
	if g.stepDelay == 0 {
		steps = 1
	}
	for i := 0; i < steps; i++ {
		_, _ = p.b.call(ctx, "Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": x, "y": y, "deltaX": 0, "deltaY": dy / float64(steps)})
		sleepCtx(ctx, g.stepDelay*5)
	}
}

// ---------------------------------------------------------------- Playwright-style locators

// locatorJS finds an element (role=, text=, label=, placeholder=, testid=, xpath= or CSS), scrolls it
// into view and returns its centre, tag, text and whether it can be interacted with.
func locatorJS(sel string) string {
	q, _ := json.Marshal(sel)
	return fmt.Sprintf(`(() => { const sel = %s;
  const vis = e => { const r = e.getBoundingClientRect(), cs = getComputedStyle(e); return r.width > 0 && r.height > 0 && cs.visibility !== 'hidden' && cs.display !== 'none' && cs.opacity !== '0'; };
  const name = e => (e.getAttribute('aria-label') || (e.labels && e.labels[0] && e.labels[0].innerText) || e.innerText || e.value || e.getAttribute('alt') || e.getAttribute('title') || e.getAttribute('placeholder') || '').trim().replace(/\s+/g, ' ');
  const pick = (list, want) => { list = list.filter(vis); if (!want) return list[0]; const w = want.toLowerCase(); return list.find(e => name(e).toLowerCase() === w) || list.filter(e => name(e).toLowerCase().includes(w)).sort((a, b) => name(a).length - name(b).length)[0]; };
  const roles = { button: 'button,[role=button],input[type=submit],input[type=button],input[type=reset]', link: 'a[href],[role=link]', textbox: 'input:not([type]),input[type=text],input[type=email],input[type=password],input[type=search],input[type=tel],input[type=url],input[type=number],textarea,[role=textbox],[contenteditable=true]', checkbox: 'input[type=checkbox],[role=checkbox]', radio: 'input[type=radio],[role=radio]', heading: 'h1,h2,h3,h4,h5,h6,[role=heading]', combobox: 'select,[role=combobox]', option: 'option,[role=option]', tab: '[role=tab]', menuitem: '[role=menuitem]', img: 'img,[role=img]', dialog: 'dialog,[role=dialog]', listitem: 'li,[role=listitem]', navigation: 'nav,[role=navigation]', switch: '[role=switch]', row: 'tr,[role=row]', cell: 'td,[role=cell]' };
  let el = null, m;
  if (sel.startsWith('text=')) el = pick([...document.querySelectorAll('a,button,[role],label,input[type=submit],input[type=button],summary,li,td,th,span,p,div,h1,h2,h3,h4,h5,h6,strong,em')], sel.slice(5).replace(/^["']|["']$/g, ''));
  else if ((m = sel.match(/^role=([\w-]+)(?:\[name=["']?(.*?)["']?\])?$/))) el = pick([...document.querySelectorAll(roles[m[1]] || '[role=' + m[1] + ']')], m[2]);
  else if (sel.startsWith('label=')) { const w = sel.slice(6).replace(/^["']|["']$/g, '').toLowerCase(); const lab = [...document.querySelectorAll('label')].find(l => l.innerText.trim().toLowerCase().includes(w)); el = lab ? (lab.control || lab.querySelector('input,select,textarea')) : document.querySelector('[aria-label*="' + CSS.escape(w) + '" i]'); }
  else if (sel.startsWith('placeholder=')) el = pick([...document.querySelectorAll('[placeholder]')].filter(e => e.placeholder.toLowerCase().includes(sel.slice(12).replace(/^["']|["']$/g, '').toLowerCase())));
  else if (sel.startsWith('testid=')) el = document.querySelector('[data-testid="' + CSS.escape(sel.slice(7).replace(/^["']|["']$/g, '')) + '"]');
  else if (sel.startsWith('xpath=') || sel.startsWith('//')) el = document.evaluate(sel.replace(/^xpath=/, ''), document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
  else el = document.querySelector(sel);
  if (!el) return null;
  window.__infuseEl = el;
  el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' }); const r = el.getBoundingClientRect();
  return { x: r.left + r.width / 2, y: r.top + r.height / 2, w: r.width, h: r.height, px: r.left + scrollX, py: r.top + scrollY, tag: el.tagName.toLowerCase(), text: name(el).slice(0, 80), visible: vis(el), disabled: !!el.disabled }; })()`, q)
}

type located struct {
	X, Y, W, H float64
	PX, PY     float64 // top-left in page coordinates (for clipped captures)
	Tag, Text  string
	Visible    bool
	Disabled   bool
}

// locate finds an element, waiting up to timeout for it to appear and become visible.
func (p *Page) locate(ctx context.Context, sel string, timeout time.Duration) (*located, error) {
	deadline := time.Now().Add(timeout)
	for {
		v, err := p.b.eval(ctx, locatorJS(sel))
		if err != nil {
			return nil, err
		}
		var l *located
		_ = json.Unmarshal(v, &l)
		if l != nil && l.Visible {
			return l, nil
		}
		if time.Now().After(deadline) {
			if l != nil {
				return nil, core.ErrMsg(core.M("browser.not_visible", "selector", sel))
			}
			return nil, core.ErrMsg(core.M("browser.not_found", "selector", sel))
		}
		sleepCtx(ctx, 150*time.Millisecond)
	}
}

// ---------------------------------------------------------------- recording (for the model, not for watching)

// A recording becomes what a model can analyse: the keyframes where the screen really changed, each a
// separate sharp image, with a timeline that ties every keyframe to the actions, console messages and
// failed requests around it, and a zoomed crop of the changed region when the change is small.

type recEvent struct {
	at time.Duration
	m  core.Msg
}

// note adds an event to the recording timeline (actions, console, network failures, navigation).
func (p *Page) note(m core.Msg) {
	p.mu.Lock()
	if p.rec {
		p.events = append(p.events, recEvent{time.Since(p.recStart), m})
	}
	p.mu.Unlock()
}

func (p *Page) startRecording() {
	p.mu.Lock()
	p.rec, p.recStart, p.frames, p.events, p.trail = true, time.Now(), nil, nil, nil
	w, h, sc := p.width, p.height, p.scale
	p.mu.Unlock()
	_, _ = p.b.call(context.Background(), "Page.startScreencast", map[string]any{"format": "jpeg", "quality": 90, "maxWidth": int(float64(w) * sc), "maxHeight": int(float64(h) * sc), "everyNthFrame": 1})
}

type keyframe struct {
	at      time.Duration
	jpeg    []byte
	changed float64           // share of the screen that changed since the previous keyframe
	regions []image.Rectangle // each separate changed area (page CSS pixels), largest first
	crops   [][]byte          // a zoom of each small changed area
}

// The change grid: each cell is a small square of the page (about 11×11 CSS px on a 1440×900 page).
const sigW, sigH = 128, 80

// signature is a small grayscale thumbnail used to measure change between frames.
func signature(im image.Image) []uint8 {
	b := im.Bounds()
	out := make([]uint8, sigW*sigH)
	for y := 0; y < sigH; y++ {
		for x := 0; x < sigW; x++ {
			// the mean of 4 samples per cell: a thin text change still moves it
			var sum uint32
			for _, o := range [4][2]int{{1, 1}, {3, 1}, {1, 3}, {3, 3}} {
				r, g, bl, _ := im.At(b.Min.X+(x*4+o[0])*b.Dx()/(sigW*4), b.Min.Y+(y*4+o[1])*b.Dy()/(sigH*4)).RGBA()
				sum += (r*299 + g*587 + bl*114) / 1000 >> 8
			}
			out[y*sigW+x] = uint8(sum / 4)
		}
	}
	return out
}

// changedCells marks the cells that differ, leaving out the masked ones (where the agent's own cursor was).
func changedCells(a, b []uint8, mask []bool) ([]bool, int) {
	out := make([]bool, len(a))
	n := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > 14 && (mask == nil || !mask[i]) {
			out[i] = true
			n++
		}
	}
	return out, n
}

// changeRegions groups changed cells into separate areas (cells up to 2 apart belong together).
func changeRegions(cells []bool) []image.Rectangle {
	seen := make([]bool, len(cells))
	var out []image.Rectangle
	for i, c := range cells {
		if !c || seen[i] {
			continue
		}
		box := image.Rect(i%sigW, i/sigW, i%sigW+1, i/sigW+1)
		queue := []int{i}
		seen[i] = true
		for len(queue) > 0 {
			j := queue[0]
			queue = queue[1:]
			x, y := j%sigW, j/sigW
			box = box.Union(image.Rect(x, y, x+1, y+1))
			for dy := -2; dy <= 2; dy++ {
				for dx := -2; dx <= 2; dx++ {
					nx, ny := x+dx, y+dy
					if nx < 0 || ny < 0 || nx >= sigW || ny >= sigH {
						continue
					}
					k := ny*sigW + nx
					if cells[k] && !seen[k] {
						seen[k] = true
						queue = append(queue, k)
					}
				}
			}
		}
		out = append(out, box)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Dx()*out[i].Dy() > out[j].Dx()*out[j].Dy() })
	return out
}

// cursorMask covers the cells the agent's cursor (and its click ripple) passed over between two frames.
func (p *Page) cursorMask(from, to time.Duration, cssW, cssH int) []bool {
	p.mu.Lock()
	trail := append([]cursorPoint{}, p.trail...)
	p.mu.Unlock()
	mask := make([]bool, sigW*sigH)
	rx, ry := 34*sigW/max(cssW, 1)+1, 34*sigH/max(cssH, 1)+1
	var before *cursorPoint
	for i := range trail {
		pt := trail[i]
		if pt.at < from-80*time.Millisecond {
			before = &trail[i]
			continue
		}
		if pt.at > to {
			break
		}
		markCursor(mask, pt, cssW, cssH, rx, ry)
	}
	if before != nil {
		markCursor(mask, *before, cssW, cssH, rx, ry)
	}
	return mask
}

func markCursor(mask []bool, pt cursorPoint, cssW, cssH, rx, ry int) {
	cx, cy := int(pt.x)*sigW/max(cssW, 1), int(pt.y)*sigH/max(cssH, 1)
	for y := cy - ry; y <= cy+ry; y++ {
		for x := cx - rx; x <= cx+rx; x++ {
			if x >= 0 && y >= 0 && x < sigW && y < sigH {
				mask[y*sigW+x] = true
			}
		}
	}
}

// stopRecording picks the keyframes, saves them in dir and returns them with the timeline.
func (p *Page) stopRecording(dir string, maxFrames int) ([]keyframe, []recEvent, time.Duration, int, error) {
	_, _ = p.b.call(context.Background(), "Page.stopScreencast", nil)
	p.mu.Lock()
	frames, events := p.frames, p.events
	p.rec, p.frames, p.events = false, nil, nil
	dur := time.Since(p.recStart)
	cssW, cssH := p.width, p.height
	p.mu.Unlock()
	defer func() { p.mu.Lock(); p.trail = nil; p.mu.Unlock() }()
	if len(frames) == 0 {
		return nil, events, dur, 0, core.ErrMsg(core.M("browser.no_frames"))
	}
	if maxFrames <= 0 || maxFrames > 24 {
		maxFrames = 12
	}
	type cand struct {
		f       recFrame
		im      image.Image
		sig     []uint8
		changed float64
		regions []image.Rectangle
	}
	var cands []cand
	for i, f := range frames {
		im, err := jpeg.Decode(bytes.NewReader(f.jpeg))
		if err != nil {
			continue
		}
		sig := signature(im)
		if len(cands) == 0 {
			cands = append(cands, cand{f, im, sig, 1, nil})
			continue
		}
		last := cands[len(cands)-1]
		cells, n := changedCells(last.sig, sig, p.cursorMask(last.f.at, f.at, cssW, cssH))
		if n == 0 {
			if i == len(frames)-1 && last.f.at != f.at {
				cands = append(cands, cand{f, im, sig, 0, nil}) // the final state, even if unchanged
			}
			continue
		}
		// a change that undoes the previous one (a caret blinking, a hover flicker) cancels it out
		if len(cands) >= 2 {
			if _, back := changedCells(cands[len(cands)-2].sig, sig, p.cursorMask(cands[len(cands)-2].f.at, f.at, cssW, cssH)); back == 0 {
				cands = cands[:len(cands)-1]
				continue
			}
		}
		cands = append(cands, cand{f, im, sig, float64(n) / float64(sigW*sigH), changeRegions(cells)})
	}
	// too many: keep the first, the last and the biggest changes, in time order
	if len(cands) > maxFrames {
		idx := make([]int, 0, len(cands)-2)
		for i := 1; i < len(cands)-1; i++ {
			idx = append(idx, i)
		}
		sort.Slice(idx, func(a, b int) bool { return cands[idx[a]].changed > cands[idx[b]].changed })
		keep := map[int]bool{0: true, len(cands) - 1: true}
		for _, i := range idx[:maxFrames-2] {
			keep[i] = true
		}
		var kept []cand
		for i, c := range cands {
			if keep[i] {
				kept = append(kept, c)
			}
		}
		cands = kept
	}
	_ = os.MkdirAll(dir, 0o755)
	var out []keyframe
	for i, c := range cands {
		kf := keyframe{at: c.f.at, jpeg: c.f.jpeg, changed: c.changed}
		b := c.im.Bounds()
		for ri, cell := range c.regions {
			if ri >= 4 {
				break
			}
			px := image.Rect(cell.Min.X*b.Dx()/sigW, cell.Min.Y*b.Dy()/sigH, cell.Max.X*b.Dx()/sigW, cell.Max.Y*b.Dy()/sigH)
			kf.regions = append(kf.regions, image.Rect(px.Min.X*cssW/b.Dx(), px.Min.Y*cssH/b.Dy(), px.Max.X*cssW/b.Dx(), px.Max.Y*cssH/b.Dy()))
			if ri < 2 && float64(px.Dx()*px.Dy())/float64(b.Dx()*b.Dy()) < 0.25 { // a small change: zoom on it
				pad := 24 * b.Dx() / max(cssW, 1)
				crop := px.Inset(-pad).Intersect(b)
				sub := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
				draw.Draw(sub, sub.Bounds(), c.im, crop.Min, draw.Src)
				var buf bytes.Buffer
				_ = jpeg.Encode(&buf, sub, &jpeg.Options{Quality: 92})
				kf.crops = append(kf.crops, buf.Bytes())
			}
		}
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%02d-%06.2fs.jpg", i+1, c.f.at.Seconds())), c.f.jpeg, 0o644)
		out = append(out, kf)
	}
	return out, events, dur, len(frames), nil
}

// ---------------------------------------------------------------- diagnostics

// networkReport lists the page's requests (only the failed ones, with onlyIssues), one per line, and
// how many it lists.
func (p *Page) networkReport(onlyIssues bool) (string, int) {
	p.mu.Lock()
	list := append([]*netEntry{}, p.net...)
	p.mu.Unlock()
	var b strings.Builder
	shown := 0
	for _, n := range list {
		issue := n.Failed != "" || n.Status >= 400
		if onlyIssues && !issue {
			continue
		}
		status := fmt.Sprint(n.Status)
		if n.Failed != "" {
			status = "FAILED " + n.Failed
		} else if !n.finished {
			status += " (pending)"
		}
		fmt.Fprintf(&b, "%s %s %s · %s · %d ms · %.1f KB\n", n.Method, status, core.Truncate(n.URL, 160), n.Type, n.Ms, n.Bytes/1024)
		shown++
	}
	return b.String(), shown
}

func (p *Page) axTree(ctx context.Context) string {
	raw, err := p.b.call(ctx, "Accessibility.getFullAXTree", nil)
	if err != nil {
		return err.Error()
	}
	var t struct {
		Nodes []struct {
			Ignored bool `json:"ignored"`
			Role    struct {
				Value string `json:"value"`
			} `json:"role"`
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
		} `json:"nodes"`
	}
	_ = json.Unmarshal(raw, &t)
	var b strings.Builder
	count := 0
	named := map[string]bool{}
	for _, n := range t.Nodes {
		role, name := n.Role.Value, strings.TrimSpace(n.Name.Value)
		if n.Ignored || role == "generic" || role == "none" || role == "InlineTextBox" || role == "LabelText" || role == "MenuListPopup" {
			continue
		}
		if role == "StaticText" && (name == "" || named[name]) {
			continue
		}
		named[name] = true
		fmt.Fprintf(&b, "%s %q\n", role, core.Truncate(n.Name.Value, 100))
		if count++; count >= 400 {
			b.WriteString("…\n")
			break
		}
	}
	return b.String()
}

var devicePresets = map[string]struct {
	w, h   int
	scale  float64
	mobile bool
	ua     string
}{
	"iphone": {390, 844, 3, true, "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"},
	"pixel":  {412, 915, 2.625, true, "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36"},
	"ipad":   {820, 1180, 2, true, "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"},
	"laptop": {1366, 768, 1, false, ""},
}

