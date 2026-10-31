package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Tray icon, menu and UTF-16 helpers.

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
