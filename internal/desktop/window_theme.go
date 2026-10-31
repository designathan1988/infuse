package desktop

import (
	"infuse/internal/core"
	"os"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

// Light/dark title bar and remembered placement.

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
	if core.ReadJSON(placementFile(), &sp) == nil && sp.W >= 400 && sp.H >= 300 {
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
	_ = core.WriteJSON(placementFile(), savedPlacement{X: r.Left, Y: r.Top, W: r.Right - r.Left, H: r.Bottom - r.Top, Maximized: wp.ShowCmd == swShowMaximized})
}

// ---- messages from the page

// IIDs and vtable slots from WebView2.h (SDK 1.0.2903).
