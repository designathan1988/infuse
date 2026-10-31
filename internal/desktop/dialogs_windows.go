//go:build windows

package desktop

// The Explorer-style open dialog: IFileOpenDialog, FOS_PICKFOLDERS for folders, FOS_ALLOWMULTISELECT
// for files. COM through vtable calls, no dependencies. Slot numbers come from shobjidl.h (IUnknown 0–2,
// IModalWindow::Show 3, then IFileDialog, then IFileOpenDialog).

import (
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32              = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
	procCoCreateInst   = ole32.NewProc("CoCreateInstance")
	procCoTaskMemFree  = ole32.NewProc("CoTaskMemFree")
)

var (
	clsidFileOpenDialog = windows.GUID{Data1: 0xDC1C5A9C, Data2: 0xE88A, Data3: 0x4DDE, Data4: [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	iidIFileOpenDialog  = windows.GUID{Data1: 0xD57C7288, Data2: 0xD4AD, Data3: 0x4768, Data4: [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
)

const (
	vtRelease    = 2
	vtShow       = 3  // IModalWindow::Show(hwndOwner)
	vtSetOptions = 9  // IFileDialog::SetOptions
	vtGetOptions = 10 // IFileDialog::GetOptions
	vtSetTitle   = 17 // IFileDialog::SetTitle
	vtGetResult  = 20 // IFileDialog::GetResult (single selection)
	vtGetResults = 27 // IFileOpenDialog::GetResults → IShellItemArray

	vtItemGetDisplayName = 5 // IShellItem::GetDisplayName
	vtArrayGetCount      = 7 // IShellItemArray::GetCount
	vtArrayGetItemAt     = 8 // IShellItemArray::GetItemAt
)

const (
	fosPickFolders      = 0x20
	fosForceFilesystem  = 0x40
	fosAllowMultiselect = 0x200
	fosPathMustExist    = 0x800
	fosFileMustExist    = 0x1000
	sigdnFileSysPath    = 0x80058000
	coinitApartment     = 0x2
	hrCancelled         = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)
)

// dialogOptions is the option set each kind of pick asks for (kept separate so it can be tested).
func dialogOptions(folders bool) uintptr {
	if folders {
		return fosPickFolders | fosForceFilesystem | fosPathMustExist
	}
	return fosAllowMultiselect | fosForceFilesystem | fosPathMustExist | fosFileMustExist
}

type winDialogs struct {
	owner func() uintptr // the main window, so the dialog is modal to it
}

func (d winDialogs) PickFolder(title string) (string, error) {
	paths, err := d.show(title, true)
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (d winDialogs) PickFiles(title string) ([]string, error) { return d.show(title, false) }

func (d winDialogs) show(title string, folders bool) ([]string, error) {
	type result struct {
		paths []string
		err   error
	}
	ch := make(chan result, 1)
	// COM wants a single-threaded apartment on the calling thread; give the dialog its own.
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		p, err := d.showSTA(title, folders)
		ch <- result{p, err}
	}()
	r := <-ch
	return r.paths, r.err
}

func (d winDialogs) showSTA(title string, folders bool) ([]string, error) {
	uninit, err := initSTA()
	if err != nil {
		return nil, err
	}
	if uninit != nil {
		defer uninit()
	}
	dialog, err := newFileDialog(title, folders)
	if err != nil {
		return nil, err
	}
	defer comCall(dialog, vtRelease)
	owner, release := d.dialogOwner()
	defer release()
	if hr := comCall(dialog, vtShow, owner); hr != 0 {
		if hr == hrCancelled {
			return nil, nil
		}
		return nil, hresultErr("Show", hr)
	}
	if folders {
		return dialogFolder(dialog)
	}
	return dialogFiles(dialog)
}

// initSTA puts the calling thread into a single-threaded apartment. The returned function undoes that;
// it is nil when COM was already initialised on the thread (nothing to undo).
func initSTA() (func(), error) {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartment)
	if hr == 0 || hr == 1 {
		return func() { procCoUninitialize.Call() }, nil
	}
	if hr != 0x80010106 { // RPC_E_CHANGED_MODE
		return nil, hresultErr("CoInitializeEx", hr)
	}
	return nil, nil
}

// newFileDialog creates the Explorer open dialog and gives it its option set and title.
func newFileDialog(title string, folders bool) (unsafe.Pointer, error) {
	var dialog unsafe.Pointer
	hr, _, _ := procCoCreateInst.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, 0x1|0x4,
		uintptr(unsafe.Pointer(&iidIFileOpenDialog)), uintptr(unsafe.Pointer(&dialog)))
	if hr != 0 || dialog == nil {
		return nil, hresultErr("CoCreateInstance(FileOpenDialog)", hr)
	}
	var opts uint32
	comCall(dialog, vtGetOptions, uintptr(unsafe.Pointer(&opts)))
	if hr := comCall(dialog, vtSetOptions, uintptr(opts)|dialogOptions(folders)); hr != 0 {
		comCall(dialog, vtRelease)
		return nil, hresultErr("SetOptions", hr)
	}
	if t, err := windows.UTF16PtrFromString(title); err == nil {
		comCall(dialog, vtSetTitle, uintptr(unsafe.Pointer(t)))
	}
	return dialog, nil
}

// dialogOwner returns the window the dialog is modal to and how to dispose of it afterwards. Opened
// from the local server (the UI in a browser) there is no such window: the dialog gets an owner of its
// own, a tiny transparent window brought to the front with the foreground thread's input attached,
// removed again when the dialog closes. A window of another process cannot own a modal dialog (Show
// returns at once, cancelled), and a background process cannot take the foreground.
func (d winDialogs) dialogOwner() (uintptr, func()) {
	if d.owner != nil {
		if owner := d.owner(); owner != 0 {
			return owner, func() {}
		}
	}
	owner := frontOwner()
	return owner, func() { procDestroyWindow.Call(owner) }
}

// dialogFolder reads the single folder the user picked.
func dialogFolder(dialog unsafe.Pointer) ([]string, error) {
	var item unsafe.Pointer
	if hr := comCall(dialog, vtGetResult, uintptr(unsafe.Pointer(&item))); hr != 0 || item == nil {
		return nil, hresultErr("GetResult", hr)
	}
	defer comCall(item, vtRelease)
	p, err := shellItemPath(item)
	if err != nil {
		return nil, err
	}
	return []string{p}, nil
}

// dialogFiles reads the files the user picked.
func dialogFiles(dialog unsafe.Pointer) ([]string, error) {
	var arr unsafe.Pointer
	if hr := comCall(dialog, vtGetResults, uintptr(unsafe.Pointer(&arr))); hr != 0 || arr == nil {
		return nil, hresultErr("GetResults", hr)
	}
	defer comCall(arr, vtRelease)
	var n uint32
	comCall(arr, vtArrayGetCount, uintptr(unsafe.Pointer(&n)))
	var out []string
	for i := uint32(0); i < n; i++ {
		var item unsafe.Pointer
		if comCall(arr, vtArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item))) != 0 || item == nil {
			continue
		}
		if p, err := shellItemPath(item); err == nil {
			out = append(out, p)
		}
		comCall(item, vtRelease)
	}
	return out, nil
}

func shellItemPath(item unsafe.Pointer) (string, error) {
	var pathPtr *uint16
	if hr := comCall(item, vtItemGetDisplayName, uintptr(sigdnFileSysPath), uintptr(unsafe.Pointer(&pathPtr))); hr != 0 || pathPtr == nil {
		return "", hresultErr("GetDisplayName", hr)
	}
	defer procCoTaskMemFree.Call(uintptr(unsafe.Pointer(pathPtr)))
	p := windows.UTF16PtrToString(pathPtr)
	if len(p) > 3 { // keep "C:\" intact
		p = strings.TrimRight(p, `\`)
	}
	return p, nil
}

// comCall invokes vtable slot `slot` of a COM object and returns the HRESULT.
func comCall(obj unsafe.Pointer, slot int, args ...uintptr) uintptr {
	vt := *(**[64]uintptr)(obj)
	full := append([]uintptr{uintptr(obj)}, args...)
	r1, _, _ := syscall.SyscallN(vt[slot], full...)
	return r1
}

func hresultErr(step string, hr uintptr) error {
	return fmt.Errorf("diálogo do sistema (%s) falhou: HRESULT 0x%08X", step, uint32(hr))
}

func OSDialogs(owner func() uintptr) NativeDialogs { return winDialogs{owner: owner} }

var (
	procGetWindowThreadProcessID   = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput          = user32.NewProc("AttachThreadInput")
	procBringWindowToTop           = user32.NewProc("BringWindowToTop")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
)

// frontOwner creates an invisible, topmost owner window at the centre of the screen and makes it the
// foreground window, so the dialog it owns opens in front of everything and with the keyboard.
func frontOwner() uintptr {
	const wsPopup, wsExTopmost, wsExToolwindow, wsExLayered = 0x80000000, 0x8, 0x80, 0x80000
	cls, _ := windows.UTF16PtrFromString("STATIC")
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	h, _, _ := procCreateWindowExW.Call(wsExTopmost|wsExToolwindow|wsExLayered, uintptr(unsafe.Pointer(cls)), 0, wsPopup,
		sw/2, sh/3, 1, 1, 0, 0, 0, 0)
	if h == 0 {
		return 0
	}
	procSetLayeredWindowAttributes.Call(h, 0, 0, 2) // LWA_ALPHA 0: fully transparent
	procShowWindow.Call(h, 5)                       // SW_SHOW
	fg, _, _ := procGetForegroundWindow.Call()
	fgThread, _, _ := procGetWindowThreadProcessID.Call(fg, 0)
	me := uintptr(windows.GetCurrentThreadId())
	if fgThread != 0 && fgThread != me {
		procAttachThreadInput.Call(me, fgThread, 1)
		defer procAttachThreadInput.Call(me, fgThread, 0)
	}
	procBringWindowToTop.Call(h)
	procSetForegroundWindow.Call(h)
	return h
}
