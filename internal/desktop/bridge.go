package desktop

// Bridge between the window's page and the OS: native dialogs, dropped paths, notifications, opening a
// folder in the file manager. The page calls `infuse.call(method, ...params)`; each window host
// (WebView2 on Windows, WKWebView/WebKitGTK elsewhere) forwards the call here. Paths never come from
// text the user types: they come from the OS dialog or from a drop.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// NativeDialogs is implemented once per OS (dialogs_*.go) and stubbed in tests.
type NativeDialogs interface {
	// PickFolder returns "" when the user cancels.
	PickFolder(title string) (string, error)
	// PickFiles returns nil when the user cancels.
	PickFiles(title string) ([]string, error)
}

// Host is what the window gives the bridge: notifications and the owner window for dialogs.
type Host interface {
	Notify(title, body string)
	Focused() bool
}

// BrowserHost is a window that can show sites in the browser pane natively (Windows: WebView2 views).
type BrowserHost interface {
	Browser(op string, a BrowserArgs) (any, error)
}

// BrowserArgs is what the page sends with each operation; the rectangle is in physical pixels of the
// window's client area.
type BrowserArgs struct {
	ID      string `json:"id"`
	URL     string `json:"url"`
	X       int32  `json:"x"`
	Y       int32  `json:"y"`
	W       int32  `json:"w"`
	H       int32  `json:"h"`
	Visible bool   `json:"visible"`
}

type Bridge struct {
	Dialogs NativeDialogs
	Host    Host
}

// DropResult splits dropped paths the way the window uses them: folders become projects, files attachments.
type DropResult struct {
	Projects    []string          `json:"projects"`
	Attachments []core.Attachment `json:"attachments"`
}

var ImageExt = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}

func AttachmentOf(p string) (core.Attachment, bool) {
	st, err := os.Stat(p)
	if err != nil || st.IsDir() {
		return core.Attachment{}, false
	}
	_, img := ImageExt[strings.ToLower(filepath.Ext(p))]
	return core.Attachment{Path: p, Name: filepath.Base(p), Size: st.Size(), Image: img}, true
}

func (b *Bridge) Classify(paths []string) DropResult {
	res := DropResult{Projects: []string{}, Attachments: []core.Attachment{}}
	for _, p := range paths {
		p = filepath.Clean(p)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			config.RememberProject(p)
			res.Projects = append(res.Projects, p)
		} else if a, ok := AttachmentOf(p); ok {
			res.Attachments = append(res.Attachments, a)
		}
	}
	return res
}

// Call dispatches one page request. Results are JSON-encodable; nil means "nothing" (e.g. cancelled).
func (b *Bridge) Call(method string, params []json.RawMessage) (any, error) {
	strArg := func(i int) string {
		if i >= len(params) {
			return ""
		}
		var s string
		_ = json.Unmarshal(params[i], &s)
		return s
	}
	switch method {
	case "pickFolder":
		p, err := b.Dialogs.PickFolder(core.M("dialog.chooseFolder").Render(config.LoadSettings().Language))
		if err != nil || p == "" {
			return nil, err
		}
		p = filepath.Clean(p)
		config.RememberProject(p)
		return p, nil
	case "pickFiles":
		list, err := b.Dialogs.PickFiles(core.M("dialog.attachFiles").Render(config.LoadSettings().Language))
		if err != nil || len(list) == 0 {
			return nil, err
		}
		out := []core.Attachment{}
		for _, p := range list {
			if a, ok := AttachmentOf(p); ok {
				out = append(out, a)
			}
		}
		return out, nil
	case "drop":
		var paths []string
		if len(params) > 0 {
			_ = json.Unmarshal(params[0], &paths)
		}
		return b.Classify(paths), nil
	case "notify":
		if b.Host != nil && !b.Host.Focused() {
			b.Host.Notify(strArg(0), strArg(1))
		}
		return nil, nil
	case "reveal":
		return nil, RevealInFileManager(strArg(0))
	case "platform":
		_, browser := b.Host.(BrowserHost)
		return map[string]any{"os": runtime.GOOS, "core.Version": core.Version, "native": true, "browser": browser}, nil
	case "browser":
		bh, ok := b.Host.(BrowserHost)
		if !ok {
			return nil, fmt.Errorf("native browser unavailable on this platform")
		}
		var a BrowserArgs
		if len(params) > 1 {
			_ = json.Unmarshal(params[1], &a)
		}
		return bh.Browser(strArg(0), a)
	}
	return nil, fmt.Errorf("unknown method: %s", method)
}

// asEscape quotes a string for AppleScript.
func asEscape(s string) string { return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) }

func RevealInFileManager(p string) error {
	st, err := os.Stat(p)
	if err != nil {
		return err
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if st.IsDir() {
			cmd = exec.Command("explorer.exe", p)
		} else {
			cmd = exec.Command("explorer.exe", "/select,", p)
		}
	case "darwin":
		cmd = exec.Command("open", "-R", p)
	default:
		if !st.IsDir() {
			p = filepath.Dir(p)
		}
		cmd = exec.Command("xdg-open", p)
	}
	return cmd.Start()
}

// bridgeScript is injected before the page loads. `post` is the host's message channel.
const bridgeScript = `(function(){
  if (window.infuse) return;
  var seq = 0, pending = {}, listeners = {};
  window.infuse = {
    native: true,
    call: function(method){ var params = Array.prototype.slice.call(arguments, 1);
      return new Promise(function(res, rej){ var id = ++seq; pending[id] = {res: res, rej: rej};
        window.__infusePost(JSON.stringify({id: id, method: method, params: params})); }); },
    drop: function(files){ var id = ++seq; return new Promise(function(res, rej){ pending[id] = {res: res, rej: rej};
        window.__infusePostFiles(JSON.stringify({id: id, method: 'drop', params: []}), files); }); },
    on: function(ev, fn){ (listeners[ev] = listeners[ev] || []).push(fn); },
    _done: function(id, ok, v){ var p = pending[id]; if (!p) return; delete pending[id]; ok ? p.res(v) : p.rej(new Error(v)); },
    _emit: function(ev, data){ (listeners[ev] || []).forEach(function(fn){ try { fn(data) } catch (e) {} }); }
  };
})();`
