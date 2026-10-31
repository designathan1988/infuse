//go:build windows

package desktop

// The application window on Windows: a top-level window hosting WebView2 (the Edge runtime that ships
// with Windows). Its own window procedure gives it: remembered size and position, a title bar that
// follows the OS light/dark theme, a tray icon (Abrir / Sair), notifications, and the paths of files
// and folders dropped on it (WebView2 hands them over with postMessageWithAdditionalObjects).

import (
	"sync"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
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
