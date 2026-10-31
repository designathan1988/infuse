//go:build windows

package main

// The application window on Windows: a top-level window hosting WebView2 (the Edge runtime that ships
// with Windows). Its own window procedure gives it: remembered size and position, a title bar that
// follows the OS light/dark theme, a tray icon (Abrir / Sair), notifications, and the paths of files
// and folders dropped on it (WebView2 hands them over with postMessageWithAdditionalObjects).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procShowWindow              = user32.NewProc("ShowWindow")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procPostMessageW            = user32.NewProc("PostMessageW")
	procPostQuitMessage         = user32.NewProc("PostQuitMessage")
	procGetWindowPlacement      = user32.NewProc("GetWindowPlacement")
	procSetWindowPlacement      = user32.NewProc("SetWindowPlacement")
	procGetForegroundWindow     = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
	procIsWindowVisible         = user32.NewProc("IsWindowVisible")
	procIsIconic                = user32.NewProc("IsIconic")
	procLoadCursorW             = user32.NewProc("LoadCursorW")
	procCreatePopupMenu         = user32.NewProc("CreatePopupMenu")
	procAppendMenuW             = user32.NewProc("AppendMenuW")
	procTrackPopupMenu          = user32.NewProc("TrackPopupMenu")
	procDestroyMenu             = user32.NewProc("DestroyMenu")
	procGetCursorPos            = user32.NewProc("GetCursorPos")
	procSendMessageW            = user32.NewProc("SendMessageW")
	procMonitorFromRect         = user32.NewProc("MonitorFromRect")
	procGetSystemMetrics        = user32.NewProc("GetSystemMetrics")
	procMessageBoxW             = user32.NewProc("MessageBoxW")
	procCreateIconFromResEx     = user32.NewProc("CreateIconFromResourceEx")
	procSetProcessDpiAwarenessC = user32.NewProc("SetProcessDpiAwarenessContext")
	procFindWindowW             = user32.NewProc("FindWindowW")

	procDwmSetWindowAttribute = windows.NewLazySystemDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	procShellNotifyIconW      = windows.NewLazySystemDLL("shell32.dll").NewProc("Shell_NotifyIconW")
)

const (
	wmDestroy        = 0x0002
	wmMove           = 0x0003
	wmSize           = 0x0005
	wmActivate       = 0x0006
	wmClose          = 0x0010
	wmSettingChange  = 0x001A
	wmGetMinMaxInfo  = 0x0024
	wmSetIcon        = 0x0080
	wmCommand        = 0x0111
	wmLButtonUp      = 0x0202
	wmLButtonDblClk  = 0x0203
	wmRButtonUp      = 0x0205
	wmApp            = 0x8000
	wmAppDispatch    = wmApp + 1
	wmAppTray        = wmApp + 2
	wmAppShow        = wmApp + 3
	ninBalloonClick  = 0x0405
	swHide           = 0
	swShowNormal     = 1
	swShowMaximized  = 3
	swShow           = 5
	swRestore        = 9
	wsOverlappedWin  = 0x00CF0000
	cwUseDefault     = 0x80000000
	nimAdd           = 0
	nimModify        = 1
	nimDelete        = 2
	nifMessage       = 0x1
	nifIcon          = 0x2
	nifTip           = 0x4
	nifInfo          = 0x10
	tpmReturnCmd     = 0x0100
	tpmRightButton   = 0x0002
	menuOpen         = 1
	menuQuit         = 2
	windowClass      = "infuse.window"
	dwmaDarkTitleBar = 20
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type windowPlacement struct {
	Length    uint32
	Flags     uint32
	ShowCmd   uint32
	MinPos    point
	MaxPos    point
	NormalPos rect
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUID            windows.GUID
	BalloonIcon     uintptr
}

type winWindow struct {
	hwnd     uintptr
	chromium *edge.Chromium
	webview  unsafe.Pointer // ICoreWebView2
	bridge   *Bridge
	icon     uintptr
	trayOn   bool
	quitting bool
	ready    bool

	mu    sync.Mutex
	queue []func()
}

var mainWin *winWindow

// savedPlacement is remembered between runs in ~/.infuse/window.json.
type savedPlacement struct {
	X, Y, W, H int32
	Maximized  bool
}

// The window, its WebView2 views and their message loop must stay on one OS thread: main() runs on the
// main thread, and init pins the main goroutine to it.
func init() { runtime.LockOSThread() }

func placementFile() string { return filepath.Join(InfuseHome(), "window.json") }

// RunWindow opens the application window on url and blocks until it closes.
func RunWindow(url string) {
	procSetProcessDpiAwarenessC.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)

	// A second launch brings the running window forward instead of opening another.
	cls, _ := windows.UTF16PtrFromString(windowClass)
	if h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(cls)), 0); h != 0 {
		procPostMessageW.Call(h, wmAppShow, 0, 0)
		return
	}

	w := &winWindow{}
	mainWin = w
	w.bridge = &Bridge{Dialogs: osDialogs(func() uintptr { return w.hwnd }), Host: w}
	w.icon = makeIcon(32)

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
		fatalBox("Não foi possível criar a janela.")
		return
	}
	w.applyTheme()
	w.restorePlacement()

	w.chromium = edge.NewChromium()
	w.chromium.DataPath = filepath.Join(InfuseHome(), "webview")
	w.chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)
	if !w.chromium.Embed(w.hwnd) {
		fatalBox("O Microsoft Edge WebView2 Runtime não está disponível neste computador.\n\nInstale-o em https://go.microsoft.com/fwlink/p/?LinkId=2124703 e abra o infuse de novo.")
		return
	}
	w.ready = true
	if settings, err := w.chromium.GetSettings(); err == nil {
		dev := os.Getenv("INFUSE_DEVTOOLS") != ""
		_ = settings.PutAreDevToolsEnabled(dev)
		_ = settings.PutIsStatusBarEnabled(false)
		_ = settings.PutIsZoomControlEnabled(false)
	}
	w.hookMessages()
	w.chromium.Init(`window.__infusePost = function(s){ window.chrome.webview.postMessage(s) };
window.__infusePostFiles = function(s, files){ window.chrome.webview.postMessageWithAdditionalObjects(s, files) };
window.addEventListener('error', function(e){ window.__infusePost(JSON.stringify({id:1, method:'log', params:['error', String(e.message), String(e.filename)+':'+e.lineno]})) });
window.addEventListener('unhandledrejection', function(e){ window.__infusePost(JSON.stringify({id:1, method:'log', params:['rejection', String(e.reason && e.reason.message || e.reason)]})) });` + bridgeScript)
	windowLog("window up: %s", url)
	w.chromium.Resize()
	w.chromium.Navigate(url)
	if !LoadSettings().HideTray && os.Getenv("INFUSE_WINDOW_HIDDEN") == "" {
		w.addTray("")
	}
	w.chromium.Focus()

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
		w.savePlacement()
		if !w.quitting && LoadSettings().CloseToTray && w.trayOn {
			procShowWindow.Call(hwnd, swHide)
			return 0
		}
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		w.removeTray()
		procPostQuitMessage.Call(0)
		return 0
	case wmAppDispatch:
		w.mu.Lock()
		q := w.queue
		w.queue = nil
		w.mu.Unlock()
		for _, f := range q {
			f()
		}
		return 0
	case wmAppShow:
		w.show()
		return 0
	case wmAppTray:
		switch lp & 0xFFFF {
		case wmLButtonUp, wmLButtonDblClk, ninBalloonClick:
			w.show()
		case wmRButtonUp:
			w.trayMenu()
		}
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// dispatch runs f on the window thread (WebView2 calls must happen there).
func (w *winWindow) dispatch(f func()) {
	w.mu.Lock()
	w.queue = append(w.queue, f)
	w.mu.Unlock()
	procPostMessageW.Call(w.hwnd, wmAppDispatch, 0, 0)
}

func (w *winWindow) show() {
	if v, _, _ := procIsIconic.Call(w.hwnd); v != 0 {
		procShowWindow.Call(w.hwnd, swRestore)
	} else {
		procShowWindow.Call(w.hwnd, swShow)
	}
	procSetForegroundWindow.Call(w.hwnd)
}

// ---- Host (for the bridge)

func (w *winWindow) Focused() bool {
	fg, _, _ := procGetForegroundWindow.Call()
	vis, _, _ := procIsWindowVisible.Call(w.hwnd)
	return fg == w.hwnd && vis != 0
}

func (w *winWindow) Notify(title, body string) {
	w.dispatch(func() {
		temporary := !w.trayOn
		if temporary {
			w.addTray("")
		}
		nid := w.nid()
		nid.Flags = nifInfo
		copyUTF16(nid.InfoTitle[:], title)
		copyUTF16(nid.Info[:], body)
		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
		if temporary {
			windows.NewLazySystemDLL("kernel32.dll") // keep the icon long enough for the toast to show
			go func() {
				timer := make(chan struct{})
				go func() { windowsSleep(10000); close(timer) }()
				<-timer
				w.dispatch(func() {
					if LoadSettings().HideTray {
						w.removeTray()
					}
				})
			}()
		}
	})
}

func windowsSleep(ms uint32) { windows.SleepEx(ms, false) }

// ---- tray

func (w *winWindow) nid() notifyIconData {
	n := notifyIconData{Wnd: w.hwnd, ID: 1}
	n.Size = uint32(unsafe.Sizeof(n))
	return n
}

func (w *winWindow) addTray(string) {
	if w.trayOn {
		return
	}
	n := w.nid()
	n.Flags = nifMessage | nifIcon | nifTip
	n.CallbackMessage = wmAppTray
	n.Icon = makeIcon(16)
	copyUTF16(n.Tip[:], "infuse")
	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&n))); r != 0 {
		w.trayOn = true
	}
}

func (w *winWindow) removeTray() {
	if !w.trayOn {
		return
	}
	n := w.nid()
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&n)))
	w.trayOn = false
}

func (w *winWindow) trayMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	open, _ := windows.UTF16PtrFromString("Abrir")
	quit, _ := windows.UTF16PtrFromString("Sair")
	procAppendMenuW.Call(menu, 0, menuOpen, uintptr(unsafe.Pointer(open)))
	procAppendMenuW.Call(menu, 0x800, 0, 0) // MF_SEPARATOR
	procAppendMenuW.Call(menu, 0, menuQuit, uintptr(unsafe.Pointer(quit)))
	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(w.hwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, w.hwnd, 0)
	procDestroyMenu.Call(menu)
	switch cmd {
	case menuOpen:
		w.show()
	case menuQuit:
		w.quitting = true
		procPostMessageW.Call(w.hwnd, wmClose, 0, 0)
	}
}

func copyUTF16(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)-1]
		u = append(u, 0)
	}
	copy(dst, u)
}

// ---- theme and placement

func osDarkMode() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	return err == nil && v == 0
}

func (w *winWindow) applyTheme() {
	var dark int32
	if osDarkMode() {
		dark = 1
	}
	procDwmSetWindowAttribute.Call(w.hwnd, dwmaDarkTitleBar, uintptr(unsafe.Pointer(&dark)), 4)
}

func (w *winWindow) restorePlacement() {
	if os.Getenv("INFUSE_WINDOW_HIDDEN") != "" { // automated end-to-end runs: full size, never shown
		procSetWindowPos.Call(w.hwnd, 0, 0, 0, 1440, 900, 0x0010|0x0004) // SWP_NOACTIVATE|SWP_NOZORDER
		return
	}
	var sp savedPlacement
	wp := windowPlacement{ShowCmd: swShowNormal}
	wp.Length = uint32(unsafe.Sizeof(wp))
	if readJSON(placementFile(), &sp) == nil && sp.W >= 400 && sp.H >= 300 {
		r := rect{sp.X, sp.Y, sp.X + sp.W, sp.Y + sp.H}
		if mon, _, _ := procMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 0); mon != 0 { // still on a screen
			wp.NormalPos = r
			if sp.Maximized {
				wp.ShowCmd = swShowMaximized
			}
			procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
			procShowWindow.Call(w.hwnd, uintptr(wp.ShowCmd))
			return
		}
	}
	// First run: centred, sized to the screen.
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	ww, wh := int32(sw)*3/4, int32(sh)*4/5
	if ww > 1500 {
		ww = 1500
	}
	x, y := (int32(sw)-ww)/2, (int32(sh)-wh)/2
	wp.NormalPos = rect{x, y, x + ww, y + wh}
	procSetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp)))
	procShowWindow.Call(w.hwnd, swShowNormal)
}

func (w *winWindow) savePlacement() {
	var wp windowPlacement
	wp.Length = uint32(unsafe.Sizeof(wp))
	if r, _, _ := procGetWindowPlacement.Call(w.hwnd, uintptr(unsafe.Pointer(&wp))); r == 0 {
		return
	}
	r := wp.NormalPos
	_ = writeJSON(placementFile(), savedPlacement{X: r.Left, Y: r.Top, W: r.Right - r.Left, H: r.Bottom - r.Top, Maximized: wp.ShowCmd == swShowMaximized})
}

// ---- messages from the page

// IIDs and vtable slots from WebView2.h (SDK 1.0.2903).
var (
	iidWebMessageArgs2  = windows.GUID{Data1: 0x06fc7ab7, Data2: 0xc90c, Data3: 0x4297, Data4: [8]byte{0x93, 0x89, 0x33, 0xca, 0x01, 0xcf, 0x6d, 0x5e}}
	iidCoreWebView2File = windows.GUID{Data1: 0xf2c19559, Data2: 0x6bc1, Data3: 0x4583, Data4: [8]byte{0xa7, 0x57, 0x90, 0x02, 0x1b, 0xe9, 0xaf, 0xec}}
)

const (
	slotQueryInterface         = 0
	slotControllerCoreWebView2 = 25 // ICoreWebView2Controller::get_CoreWebView2
	slotAddWebMessageReceived  = 34 // ICoreWebView2::add_WebMessageReceived
	slotTryGetMessageAsString  = 5  // ICoreWebView2WebMessageReceivedEventArgs
	slotAdditionalObjects      = 6  // ICoreWebView2WebMessageReceivedEventArgs2
	slotCollectionCount        = 3  // ICoreWebView2ObjectCollectionView
	slotCollectionAt           = 4
	slotFilePath               = 3 // ICoreWebView2File::get_Path
)

// comHandler is a minimal COM object implementing ICoreWebView2WebMessageReceivedEventHandler.
type comHandler struct{ vtbl *[4]uintptr }

var (
	messageHandlerVtbl = [4]uintptr{
		windows.NewCallback(func(this, riid, out uintptr) uintptr { *(*uintptr)(ptrOf(out)) = this; return 0 }),
		windows.NewCallback(func(this uintptr) uintptr { return 1 }),
		windows.NewCallback(func(this uintptr) uintptr { return 1 }),
		windows.NewCallback(func(this, sender, args uintptr) uintptr { mainWin.onMessage(ptrOf(args)); return 0 }),
	}
	messageHandler = &comHandler{vtbl: &messageHandlerVtbl}
)

func (w *winWindow) hookMessages() {
	ctrl := w.chromium.GetController()
	if ctrl == nil {
		return
	}
	comCall(unsafe.Pointer(ctrl), slotControllerCoreWebView2, uintptr(unsafe.Pointer(&w.webview)))
	if w.webview == nil {
		return
	}
	var token int64
	comCall(w.webview, slotAddWebMessageReceived, uintptr(unsafe.Pointer(messageHandler)), uintptr(unsafe.Pointer(&token)))
}

type pageCall struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

// windowLog records bridge traffic and page errors in ~/.infuse/window.log (the window has no console).
func windowLog(format string, a ...any) {
	f, err := os.OpenFile(filepath.Join(InfuseHome(), "window.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, time.Now().Format("15:04:05.000 ")+format+"\n", a...)
}

func (w *winWindow) onMessage(args unsafe.Pointer) {
	var p *uint16
	if hr := comCall(args, slotTryGetMessageAsString, uintptr(unsafe.Pointer(&p))); hr != 0 || p == nil {
		windowLog("message: TryGetWebMessageAsString hr=0x%08X", uint32(hr))
		return
	}
	raw := windows.UTF16PtrToString(p)
	windows.CoTaskMemFree(unsafe.Pointer(p))
	var c pageCall
	if json.Unmarshal([]byte(raw), &c) != nil || c.ID == 0 || c.Method == "" {
		windowLog("message ignored: %s", truncate(raw, 200))
		return
	}
	if c.Method == "log" { // page-side errors and traces
		windowLog("page: %s", truncate(raw, 1000))
		return
	}
	windowLog("call %d %s", c.ID, c.Method)
	if c.Method == "drop" {
		b, _ := json.Marshal(droppedPaths(args))
		c.Params = []json.RawMessage{b}
	}
	go func() {
		res, err := w.bridge.Call(c.Method, c.Params)
		var js string
		if err != nil {
			e, _ := json.Marshal(err.Error())
			js = fmt.Sprintf("window.infuse && infuse._done(%d,false,%s)", c.ID, e)
		} else {
			b, _ := json.Marshal(res)
			js = fmt.Sprintf("window.infuse && infuse._done(%d,true,%s)", c.ID, b)
		}
		windowLog("result %d %s → %s", c.ID, c.Method, truncate(js, 300))
		w.dispatch(func() { w.chromium.Eval(js) })
	}()
}

// droppedPaths reads the File objects that came with a postMessageWithAdditionalObjects call.
func droppedPaths(args unsafe.Pointer) []string {
	var args2 unsafe.Pointer
	if comCall(args, slotQueryInterface, uintptr(unsafe.Pointer(&iidWebMessageArgs2)), uintptr(unsafe.Pointer(&args2))) != 0 || args2 == nil {
		return nil
	}
	defer comCall(args2, vtRelease)
	var coll unsafe.Pointer
	if comCall(args2, slotAdditionalObjects, uintptr(unsafe.Pointer(&coll))) != 0 || coll == nil {
		return nil
	}
	defer comCall(coll, vtRelease)
	var n uint32
	comCall(coll, slotCollectionCount, uintptr(unsafe.Pointer(&n)))
	var out []string
	for i := uint32(0); i < n; i++ {
		var obj unsafe.Pointer
		if comCall(coll, slotCollectionAt, uintptr(i), uintptr(unsafe.Pointer(&obj))) != 0 || obj == nil {
			continue
		}
		var file unsafe.Pointer
		if comCall(obj, slotQueryInterface, uintptr(unsafe.Pointer(&iidCoreWebView2File)), uintptr(unsafe.Pointer(&file))) == 0 && file != nil {
			var p *uint16
			if comCall(file, slotFilePath, uintptr(unsafe.Pointer(&p))) == 0 && p != nil {
				out = append(out, windows.UTF16PtrToString(p))
				windows.CoTaskMemFree(unsafe.Pointer(p))
			}
			comCall(file, vtRelease)
		}
		comCall(obj, vtRelease)
	}
	return out
}

// ---- icon: drawn at runtime (PNG handed to CreateIconFromResourceEx), neutral greys.

func makeIcon(size int) uintptr {
	png := iconPNG(size)
	h, _, _ := procCreateIconFromResEx.Call(uintptr(unsafe.Pointer(&png[0])), uintptr(len(png)), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	return h
}

var _ = strconv.Itoa
var _ = procSendMessageW

// ptrOf converts a pointer the OS handed us as uintptr (window-procedure and COM callback parameters).
func ptrOf(v uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&v)) }
