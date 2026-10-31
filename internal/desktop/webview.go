package desktop

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strconv"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WebView2 message hook, page calls and dropped paths.

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
	f, err := os.OpenFile(filepath.Join(core.InfuseHome(), "window.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
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
		windowLog("message ignored: %s", core.Truncate(raw, 200))
		return
	}
	if c.Method == "log" { // page-side errors and traces
		windowLog("page: %s", core.Truncate(raw, 1000))
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
		windowLog("result %d %s → %s", c.ID, c.Method, core.Truncate(js, 300))
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
	png := IconPNG(size)
	h, _, _ := procCreateIconFromResEx.Call(uintptr(unsafe.Pointer(&png[0])), uintptr(len(png)), 1, 0x00030000, uintptr(size), uintptr(size), 0)
	return h
}

var _ = strconv.Itoa
var _ = procSendMessageW

// ptrOf converts a pointer the OS handed us as uintptr (window-procedure and COM callback parameters).
func ptrOf(v uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&v)) }
