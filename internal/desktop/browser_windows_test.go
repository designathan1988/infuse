//go:build windows

package desktop

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procPeekMessageW = user32.NewProc("PeekMessageW")

// The native browser pane, in a window that is never shown: a site that refuses iframes
// (X-Frame-Options) loads, its address and title come back as events, and back/close work.
func testDesktopHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func TestNativeBrowserLoadsAnySite(t *testing.T) {
	if os.Getenv("INFUSE_LIVE") == "" {
		t.Skip("INFUSE_LIVE not set (needs WebView2 and the network)")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	testDesktopHome(t)
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)
	cls, _ := windows.UTF16PtrFromString("STATIC")
	hwnd, _, _ := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), 0, wsOverlappedWin, 0, 0, 1000, 700, 0, 0, 0, 0) // no WS_VISIBLE
	if hwnd == 0 {
		t.Fatal("window")
	}
	defer procDestroyWindow.Call(hwnd)
	w := &winWindow{hwnd: hwnd}
	saved := mainWin
	mainWin = w
	defer func() { mainWin = saved; browserEventHook = nil }()

	type ev struct {
		url, title string
		loading    bool
	}
	var got []ev
	browserEventHook = func(id, url, title string, loading bool) { got = append(got, ev{url, title, loading}) }
	pumpFor := func(d time.Duration, until func() bool) bool {
		deadline := time.Now().Add(d)
		var m winMsg
		for time.Now().Before(deadline) {
			for {
				r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
				if r == 0 {
					break
				}
				procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
				procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
			}
			if until() {
				return true
			}
			time.Sleep(20 * time.Millisecond)
		}
		return false
	}
	pump := func(until func() bool) bool { return pumpFor(30*time.Second, until) }
	loaded := func(host string) func() bool {
		return func() bool {
			for _, e := range got {
				if !e.loading && strings.Contains(e.url, host) {
					return true
				}
			}
			return false
		}
	}
	if _, err := w.browserOp("open", BrowserArgs{ID: "t1", URL: "https://github.com/", X: 0, Y: 0, W: 900, H: 600, Visible: true}); err != nil {
		t.Fatal(err)
	}
	if !pump(loaded("github.com")) {
		t.Fatalf("github.com did not load: %+v", got)
	}
	if _, err := w.browserOp("navigate", BrowserArgs{ID: "t1", URL: "https://www.google.com/search?q=infuse"}); err != nil {
		t.Fatal(err)
	}
	if !pump(loaded("google.")) {
		t.Fatalf("google did not load: %+v", got)
	}
	last := got[len(got)-1]
	t.Logf("last: %s | %s", last.url, last.title)
	got = nil
	_, _ = w.browserOp("back", BrowserArgs{ID: "t1"})
	if !pump(loaded("github.com")) {
		t.Fatalf("back did not return to github: %+v", got)
	}
	// element picker: a page message counts only while the user is picking
	var picked string
	browserPickHook = func(p string) { picked = p }
	defer func() { browserPickHook = nil }()
	tb := browserTabs["t1"]
	tb.ch.Eval(`window.chrome.webview.postMessage(JSON.stringify({infusePick:{selector:"forjado"}}))`)
	pumpFor(1500*time.Millisecond, func() bool { return false })
	if picked != "" {
		t.Fatal("a page message was taken as a pick without the user picking")
	}
	if _, err := w.browserOp("pick", BrowserArgs{ID: "t1"}); err != nil {
		t.Fatal(err)
	}
	tb.ch.Eval(`setTimeout(() => { const el = document.querySelector('a, button, h1, div'); el.dispatchEvent(new MouseEvent('mousemove', {bubbles: true})); el.dispatchEvent(new MouseEvent('click', {bubbles: true, cancelable: true})); }, 300)`)
	if !pump(func() bool { return picked != "" }) || !strings.Contains(picked, `"selector"`) || !strings.Contains(picked, `"html"`) {
		t.Fatalf("pick: %q", picked)
	}
	if _, err := w.browserOp("close", BrowserArgs{ID: "t1"}); err != nil || browserTabs["t1"] != nil {
		t.Fatal("close", err)
	}
}
