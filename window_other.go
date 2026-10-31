//go:build !windows && cgo

package main

// macOS (WKWebView) and Linux (WebKitGTK) through webview_go. The bridge is bound as `__infuseCall`;
// dropped paths come from the page (WebKit exposes no filesystem path for dropped files, so on these
// systems a drop of a folder opens the native dialog positioned at it is not possible — the page falls
// back to pickFolder()).

import (
	"encoding/json"
	"os/exec"
	"runtime"

	webview "github.com/webview/webview_go"
)

type otherWindow struct{ w webview.WebView }

func (o otherWindow) Focused() bool { return true }
func (o otherWindow) Notify(title, body string) {
	if runtime.GOOS == "darwin" {
		_ = exec.Command("osascript", "-e", `display notification "`+asEscape(body)+`" with title "`+asEscape(title)+`"`).Start()
		return
	}
	_ = exec.Command("notify-send", title, body).Start()
}

func RunWindow(url string) {
	w := webview.New(false)
	defer w.Destroy()
	host := otherWindow{w}
	b := &Bridge{Dialogs: osDialogs(nil), Host: host}
	w.SetTitle("infuse")
	w.SetSize(1280, 820, webview.HintNone)
	_ = w.Bind("__infuseCall", func(raw string) (any, error) {
		var c struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		_ = json.Unmarshal([]byte(raw), &c)
		return b.Call(c.Method, c.Params)
	})
	w.Init(`window.__infusePost = function(s){ var m = JSON.parse(s); __infuseCall(s).then(function(v){ infuse._done(m.id, true, v) }, function(e){ infuse._done(m.id, false, String(e)) }) };
window.__infusePostFiles = function(s){ window.__infusePost(s) };` + bridgeScript)
	w.Navigate(url)
	w.Run()
}
