package browser

// The agent's browser, made to look and act like a person using it: a visible cursor that travels along
// curved, accelerating paths, clicks with a pause and a ripple, typing key by key at a varying rhythm,
// scrolling in steps; screenshots at the screen density the user sets; recordings handed to the model as
// the keyframes where the screen changed, with a timeline of actions, console and network between them
// (a model reads still frames, not video); and the diagnostics a developer opens DevTools for — network,
// console, performance, accessibility tree, storage, device and network emulation. Locators follow
// Playwright's: role=, text=, label=, placeholder=, testid=, xpath= or CSS.

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"math"
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
func (p *Page) Eval(ctx context.Context, js string) (json.RawMessage, error) {
	return p.b.eval(ctx, js)
}

func (p *Page) setMetrics(w, h int, scale float64, mobile bool) {
	_, _ = p.b.call(context.Background(), "Emulation.setDeviceMetricsOverride", map[string]any{"width": w, "height": h, "deviceScaleFactor": scale, "mobile": mobile})
	p.mu.Lock()
	p.width, p.height, p.scale = w, h, scale
	p.mu.Unlock()
}
