//go:build windows

package desktop

import (
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"runtime"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// Top-level window creation, message loop and procedure.

func init() { runtime.LockOSThread() }

func placementFile() string { return filepath.Join(core.InfuseHome(), "window.json") }

// RunWindow opens the application window on url and blocks until it closes.
func RunWindow(url string) {
	procSetProcessDpiAwarenessC.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)

	// A second launch brings the running window forward instead of opening another.
	if bringExistingWindowForward() {
		return
	}
	w, ok := newMainWindow()
	if !ok {
		return
	}
	if !w.attachWebView() {
		return
	}
	w.startPage(url)
	runMessageLoop()
}

// bringExistingWindowForward asks a running window to show itself; false when none was found.
func bringExistingWindowForward() bool {
	cls, _ := windows.UTF16PtrFromString(windowClass)
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0)
	if h == 0 {
		return false
	}
	procPostMessageW.Call(h, wmAppShow, 0, 0)
	return true
}

// newMainWindow registers the window class and creates the top-level window; false when that fails.
func newMainWindow() (*winWindow, bool) {
	w := &winWindow{}
	mainWin = w
	w.bridge = &Bridge{Dialogs: OSDialogs(func() uintptr { return w.hwnd }), Host: w}
	w.icon = makeIcon(32)
	cls, _ := windows.UTF16PtrFromString(windowClass)

	var inst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &inst)
	cursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW
	wc := wndClassEx{
		Size: uint32(unsafe.Sizeof(wndClassEx{})), WndProc: windows.NewCallback(wndProc), Instance: inst,
		Icon: windows.Handle(w.icon), IconSm: windows.Handle(makeIcon(16)), Cursor: windows.Handle(cursor), ClassName: cls,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := windows.UTF16PtrFromString("infuse")
	w.hwnd, _, _ = procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(title)), wsOverlappedWin,
		cwUseDefault, cwUseDefault, 1280, 820, 0, 0, uintptr(inst), 0)
	if w.hwnd == 0 {
		fatalBox(core.M("window.createFailed").Render(config.LoadSettings().Language))
		return nil, false
	}
	w.applyTheme()
	w.restorePlacement()
	return w, true
}

// attachWebView embeds the WebView2 control in the window and applies its settings.
func (w *winWindow) attachWebView() bool {
	w.chromium = edge.NewChromium()
	w.chromium.DataPath = filepath.Join(core.InfuseHome(), "webview")
	w.chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)
	if !w.chromium.Embed(w.hwnd) {
		fatalBox(core.M("window.webviewFailed").Render(config.LoadSettings().Language))
		return false
	}
	w.ready = true
	if settings, err := w.chromium.GetSettings(); err == nil {
		dev := os.Getenv("INFUSE_DEVTOOLS") != ""
		_ = settings.PutAreDevToolsEnabled(dev)
		_ = settings.PutIsStatusBarEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
	}
	w.hookMessages()
	return true
}

// startPage injects the bridge, loads url and shows the window's chrome.
func (w *winWindow) startPage(url string) {
	w.chromium.Init(`window.__infuseBrowserHost = true;
window.__infusePost = function(s){ window.chrome.webview.postMessage(s) };
window.__infusePostFiles = function(s, files){ window.chrome.webview.postMessageWithAdditionalObjects(s, files) };
window.addEventListener('error', function(e){ window.__infusePost(JSON.stringify({id:1, method:'log', params:['error', String(e.message), String(e.filename)+':'+e.lineno]})) });
window.addEventListener('unhandledrejection', function(e){ window.__infusePost(JSON.stringify({id:1, method:'log', params:['rejection', String(e.reason && e.reason.message || e.reason)]})) });` + bridgeScript)
	windowLog("window up: %s", url)
	w.chromium.Resize()
	w.chromium.Navigate(url)
	if !config.LoadSettings().HideTray && os.Getenv("INFUSE_WINDOW_HIDDEN") == "" {
		w.addTray("")
	}
	w.chromium.Focus()
}

// runMessageLoop pumps the window's messages until it closes.
func runMessageLoop() {
	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func fatalBox(text string) {
	t, _ := windows.UTF16PtrFromString(text)
	c, _ := windows.UTF16PtrFromString("infuse")
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10)
}

func wndProc(hwnd, msg, wp, lp uintptr) uintptr {
	w := mainWin
	if w == nil || (w.hwnd != 0 && hwnd != w.hwnd) {
		r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
		return r
	}
	switch msg {
	case wmSize:
		if w.ready {
			w.chromium.Resize()
		}
	case wmMove:
		if w.ready {
			_ = w.chromium.NotifyParentWindowPositionChanged()
		}
	case wmActivate:
		if w.ready && wp&0xFFFF != 0 {
			w.chromium.Focus()
		}
	case wmGetMinMaxInfo:
		mmi := (*[5]point)(ptrOf(lp)) // ptReserved, ptMaxSize, ptMaxPosition, ptMinTrackSize, ptMaxTrackSize
		mmi[3] = point{720, 480}
		return 0
	case wmSettingChange:
		if lp != 0 && windows.UTF16PtrToString((*uint16)(ptrOf(lp))) == "ImmersiveColorSet" {
			w.applyTheme()
		}
	case wmClose:
		return w.closeWindow(hwnd)
	case wmDestroy:
		w.removeTray()
		procPostQuitMessage.Call(0)
		return 0
	case wmAppDispatch:
		w.flushQueue()
		return 0
	case wmAppShow:
		w.show()
		return 0
	case wmAppTray:
		w.trayEvent(lp)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// closeWindow saves the placement and hides to the tray when it is on, otherwise destroys the window.
func (w *winWindow) closeWindow(hwnd uintptr) uintptr {
	w.savePlacement()
	if !w.quitting && config.LoadSettings().CloseToTray && w.trayOn {
		procShowWindow.Call(hwnd, swHide)
		return 0
	}
	procDestroyWindow.Call(hwnd)
	return 0
}

// flushQueue runs the functions dispatch queued on the window thread.
func (w *winWindow) flushQueue() {
	w.mu.Lock()
	q := w.queue
	w.queue = nil
	w.mu.Unlock()
	for _, f := range q {
		f()
	}
}

// trayEvent handles a click on the tray icon.
func (w *winWindow) trayEvent(lp uintptr) {
	switch lp & 0xFFFF {
	case wmLButtonUp, wmLButtonDblClk, ninBalloonClick:
		w.show()
	case wmRButtonUp:
		w.trayMenu()
	}
}

// dispatch runs f on the window thread (WebView2 calls must happen there).
