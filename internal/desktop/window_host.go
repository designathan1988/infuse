package desktop

import (
	"infuse/internal/config"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Host operations: dispatch to the UI thread, focus and notifications.

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
					if config.LoadSettings().HideTray {
						w.removeTray()
					}
				})
			}()
		}
	})
}

func windowsSleep(ms uint32) { windows.SleepEx(ms, false) }

// ---- tray
