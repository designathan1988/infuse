//go:build windows

package desktop

// The browser pane inside the window. Each tab is its own WebView2 in a child window placed exactly
// over the pane's rectangle (sent by the page), so every site opens: an iframe is refused by most sites
// (X-Frame-Options / frame-ancestors). Links that ask for a new window open in the same tab, never in a
// window of their own. Address, title and loading state go back to the page as "browser" events.

import (
	"encoding/json"
	"errors"
	"infuse/internal/core"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// ICoreWebView2 slots (WebView2.h); the event-args slots are those of each args interface.
const (
	slotGetSource               = 4
	slotNavigate                = 5
	slotAddNavigationStarting   = 7
	slotAddSourceChanged        = 11
	slotReload                  = 31
	slotGoBack                  = 40
	slotGoForward               = 41
	slotStop                    = 43
	slotAddNewWindowRequested   = 44
	slotAddDocumentTitleChanged = 46
	slotGetDocumentTitle        = 48
	slotNewWindowGetURI         = 3 // ICoreWebView2NewWindowRequestedEventArgs::get_Uri
	slotNewWindowPutHandled     = 6 // ICoreWebView2NewWindowRequestedEventArgs::put_Handled
)

type browserTab struct {
	id      string
	hwnd    uintptr
	ch      *edge.Chromium
	wv      unsafe.Pointer // ICoreWebView2
	loading bool
	picking bool // the element picker is active: the page's next message is the user's choice
}

// pickerJS highlights the element under the mouse; a click sends its selector and HTML back, Escape cancels.
const pickerJS = `(() => { if (window.__infusePick) return; window.__infusePick = true;
  const box = document.createElement('div'); box.style.cssText = 'position:fixed;z-index:2147483647;pointer-events:none;border:2px solid #4c6fd6;background:rgba(76,111,214,.12);border-radius:3px;transition:all .05s';
  const tag = document.createElement('div'); tag.style.cssText = 'position:fixed;z-index:2147483647;pointer-events:none;background:#4c6fd6;color:#fff;font:12px system-ui;padding:2px 6px;border-radius:3px';
  document.documentElement.append(box, tag); let cur = null;
  const sel = el => { if (el.id) return '#' + CSS.escape(el.id); const p = []; for (let e = el; e && e.nodeType === 1 && e !== document.documentElement; e = e.parentElement) { let s = e.tagName.toLowerCase(); if (e.id) { p.unshift('#' + CSS.escape(e.id)); break; } const cls = [...e.classList].slice(0, 2).map(c => '.' + CSS.escape(c)).join(''); const sib = e.parentElement ? [...e.parentElement.children].filter(x => x.tagName === e.tagName) : []; s += cls + (sib.length > 1 ? ':nth-of-type(' + (sib.indexOf(e) + 1) + ')' : ''); p.unshift(s); } return p.join(' > '); };
  const move = e => { cur = e.target; const r = cur.getBoundingClientRect(); Object.assign(box.style, { left: r.left + 'px', top: r.top + 'px', width: r.width + 'px', height: r.height + 'px' }); tag.textContent = cur.tagName.toLowerCase() + (cur.id ? '#' + cur.id : '') + ' ' + Math.round(r.width) + '×' + Math.round(r.height); Object.assign(tag.style, { left: r.left + 'px', top: Math.max(0, r.top - 22) + 'px' }); };
  const done = res => { removeEventListener('mousemove', move, true); removeEventListener('click', click, true); removeEventListener('keydown', key, true); box.remove(); tag.remove(); window.__infusePick = false; window.chrome.webview.postMessage(JSON.stringify({ infusePick: res })); };
  const click = e => { e.preventDefault(); e.stopPropagation(); const el = cur || e.target; const r = el.getBoundingClientRect(); const cs = getComputedStyle(el);
    done({ url: location.href, title: document.title, selector: sel(el), html: el.outerHTML.slice(0, 4000), text: (el.innerText || '').trim().slice(0, 500), rect: { x: Math.round(r.left), y: Math.round(r.top), w: Math.round(r.width), h: Math.round(r.height) }, style: { color: cs.color, background: cs.backgroundColor, font: cs.fontSize + ' ' + cs.fontFamily.split(',')[0], margin: cs.margin, padding: cs.padding } }); };
  const key = e => { if (e.key === 'Escape') { e.preventDefault(); done(null); } };
  addEventListener('mousemove', move, true); addEventListener('click', click, true); addEventListener('keydown', key, true); })()`

var (
	browserTabs      = map[string]*browserTab{}  // touched on the window thread only
	browserBySender  = map[uintptr]*browserTab{} // ICoreWebView2 → tab, for the event handlers
	browserClass     = "infuse.browser"
	browserClassOK   bool
	procSetWindowPos = user32.NewProc("SetWindowPos")

	childProc = windows.NewCallback(func(hwnd, msg, wp, lp uintptr) uintptr {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	})
	browserEventVtbl = func(f func(sender, args uintptr)) *comHandler {
		vt := [4]uintptr{
			windows.NewCallback(func(this, riid, out uintptr) uintptr { *(*uintptr)(ptrOf(out)) = this; return 0 }),
			windows.NewCallback(func(this uintptr) uintptr { return 1 }),
			windows.NewCallback(func(this uintptr) uintptr { return 1 }),
			windows.NewCallback(func(this, sender, args uintptr) uintptr { f(sender, args); return 0 }),
		}
		return &comHandler{vtbl: &vt}
	}
	onSourceChanged = browserEventVtbl(func(sender, _ uintptr) {
		if tb := browserBySender[sender]; tb != nil {
			tb.emit()
		}
	})
	onTitleChanged = browserEventVtbl(func(sender, _ uintptr) {
		if tb := browserBySender[sender]; tb != nil {
			tb.emit()
		}
	})
	onNavigationStarting = browserEventVtbl(func(sender, _ uintptr) {
		if tb := browserBySender[sender]; tb != nil {
			tb.loading = true
			tb.emit()
		}
	})
	// window.open / target=_blank: open in the same tab instead of a separate window
	onNewWindow = browserEventVtbl(func(sender, args uintptr) {
		tb := browserBySender[sender]
		var p *uint16
		comCall(ptrOf(args), slotNewWindowGetURI, uintptr(unsafe.Pointer(&p)))
		comCall(ptrOf(args), slotNewWindowPutHandled, 1)
		if p == nil || tb == nil {
			return
		}
		uri := windows.UTF16PtrToString(p)
		windows.CoTaskMemFree(unsafe.Pointer(p))
		tb.ch.Navigate(uri)
	})
)

func (tb *browserTab) str(slot int) string {
	var p *uint16
	if comCall(tb.wv, slot, uintptr(unsafe.Pointer(&p))) != 0 || p == nil {
		return ""
	}
	s := windows.UTF16PtrToString(p)
	windows.CoTaskMemFree(unsafe.Pointer(p))
	return s
}

// browserPickHook sees the picker's answer (tests).
var browserPickHook func(pick string)

// browserEventHook sees every event (tests).
var browserEventHook func(id, url, title string, loading bool)

func (tb *browserTab) emit() {
	src, title := tb.str(slotGetSource), tb.str(slotGetDocumentTitle)
	if browserEventHook != nil {
		browserEventHook(tb.id, src, title, tb.loading)
	}
	if mainWin == nil || mainWin.chromium == nil {
		return
	}
	b, _ := json.Marshal(map[string]any{"id": tb.id, "url": src, "title": title, "loading": tb.loading})
	mainWin.chromium.Eval("window.infuse && infuse._emit('browser'," + string(b) + ")")
}

func (tb *browserTab) place(a BrowserArgs) {
	const swpNoActivate, swpShow, swpHide = 0x0010, 0x0040, 0x0080
	flags := uintptr(swpNoActivate | swpShow)
	if !a.Visible || a.W <= 0 || a.H <= 0 {
		flags = swpNoActivate | swpHide
	}
	procSetWindowPos.Call(tb.hwnd, 0, uintptr(a.X), uintptr(a.Y), uintptr(max(a.W, 1)), uintptr(max(a.H, 1)), flags)
	tb.ch.Resize()
}

// Browser runs a browser-pane operation on the window thread and waits for it.
func (w *winWindow) Browser(op string, a BrowserArgs) (any, error) {
	type result struct {
		v   any
		err error
	}
	done := make(chan result, 1)
	w.dispatch(func() { v, err := w.browserOp(op, a); done <- result{v, err} })
	select {
	case r := <-done:
		return r.v, r.err
	case <-time.After(30 * time.Second):
		return nil, errors.New("o navegador não respondeu")
	}
}

func (w *winWindow) browserOp(op string, a BrowserArgs) (any, error) {
	tb := browserTabs[a.ID]
	if op == "open" {
		if tb == nil {
			var err error
			if tb, err = w.newBrowserTab(a.ID); err != nil {
				return nil, err
			}
		}
		tb.place(a)
		if a.URL != "" {
			tb.loading = true
			tb.ch.Navigate(a.URL)
		}
		return true, nil
	}
	if tb == nil {
		return nil, errors.New("aba do navegador inexistente")
	}
	switch op {
	case "place":
		tb.place(a)
	case "navigate":
		tb.loading = true
		tb.ch.Navigate(a.URL)
	case "back":
		comCall(tb.wv, slotGoBack)
	case "forward":
		comCall(tb.wv, slotGoForward)
	case "reload":
		comCall(tb.wv, slotReload)
	case "stop":
		comCall(tb.wv, slotStop)
	case "focus":
		tb.ch.Focus()
	case "pick":
		tb.picking = true
		tb.ch.Eval(pickerJS)
		tb.ch.Focus()
	case "close":
		delete(browserTabs, tb.id)
		delete(browserBySender, uintptr(tb.wv))
		procDestroyWindow.Call(tb.hwnd)
	default:
		return nil, errors.New("operação desconhecida: " + op)
	}
	return true, nil
}

func (w *winWindow) newBrowserTab(id string) (*browserTab, error) {
	var inst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &inst)
	cls, _ := windows.UTF16PtrFromString(browserClass)
	if !browserClassOK {
		wc := wndClassEx{Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: childProc, Instance: inst, ClassName: cls}
		procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		browserClassOK = true
	}
	const wsChild, wsClipSiblings, wsClipChildren = 0x40000000, 0x04000000, 0x02000000
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, wsChild|wsClipSiblings|wsClipChildren, 0, 0, 1, 1, w.hwnd, 0, uintptr(inst), 0)
	if hwnd == 0 {
		return nil, errors.New("não foi possível criar a área do navegador")
	}
	ch := edge.NewChromium()
	ch.DataPath = filepath.Join(core.InfuseHome(), "browser") // the sites' cookies and storage, apart from the app's
	if !ch.Embed(hwnd) {
		procDestroyWindow.Call(hwnd)
		return nil, errors.New("WebView2 indisponível")
	}
	if s, err := ch.GetSettings(); err == nil {
		_ = s.PutAreDevToolsEnabled(true) // F12 inspects the page
		_ = s.PutIsStatusBarEnabled(true)
	}
	tb := &browserTab{id: id, hwnd: hwnd, ch: ch}
	ch.MessageCallback = func(msg string) { // only the picker's answer, and only while the user is picking
		var m struct {
			Pick json.RawMessage `json:"infusePick"`
		}
		if !tb.picking || json.Unmarshal([]byte(msg), &m) != nil || m.Pick == nil {
			return
		}
		tb.picking = false
		if browserPickHook != nil {
			browserPickHook(string(m.Pick))
		}
		if mainWin != nil && mainWin.chromium != nil {
			mainWin.chromium.Eval("window.infuse && infuse._emit('browserPick'," + string(m.Pick) + ")")
		}
	}
	ch.NavigationCompletedCallback = func(*edge.ICoreWebView2, *edge.ICoreWebView2NavigationCompletedEventArgs) {
		tb.loading = false
		tb.emit()
	}
	ctrl := ch.GetController()
	if ctrl == nil {
		procDestroyWindow.Call(hwnd)
		return nil, errors.New("WebView2 sem controlador")
	}
	comCall(unsafe.Pointer(ctrl), slotControllerCoreWebView2, uintptr(unsafe.Pointer(&tb.wv)))
	if tb.wv == nil {
		procDestroyWindow.Call(hwnd)
		return nil, errors.New("WebView2 sem página")
	}
	var token int64
	for slot, h := range map[int]*comHandler{slotAddSourceChanged: onSourceChanged, slotAddDocumentTitleChanged: onTitleChanged, slotAddNavigationStarting: onNavigationStarting, slotAddNewWindowRequested: onNewWindow} {
		comCall(tb.wv, slot, uintptr(unsafe.Pointer(h)), uintptr(unsafe.Pointer(&token)))
	}
	browserTabs[id] = tb
	browserBySender[uintptr(tb.wv)] = tb
	return tb, nil
}
