package main

// HTTP/SSE adapter for the browser pane outside the app window. The tabs and their screencast live in
// internal/browser.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/browser"
	"infuse/internal/core"
	"net/http"
	"time"
)

func viewAPI(w http.ResponseWriter, r *http.Request, root, path string, body map[string]any) bool {
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) float64 { f, _ := body[k].(float64); return f }
	id := str("id")
	if id == "" {
		id = r.URL.Query().Get("id")
	}
	switch path {
	case "/api/view/stream":
		t, err := browser.ViewTab(root, id, 1024, 700)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return true
		}
		fl, ok := w.(http.Flusher)
		if !ok {
			return true
		}
		w.Header().Set("content-type", "text/event-stream")
		w.Header().Set("cache-control", "no-cache")
		frame, url, title, ch, detach := t.Attach()
		defer detach()
		nav, _ := json.Marshal(map[string]any{"type": "nav", "url": url, "title": title})
		fmt.Fprintf(w, "data: %s\n\n", nav)
		if frame != nil {
			fmt.Fprintf(w, "data: %s\n\n", frame)
		}
		fl.Flush()
		ping := time.NewTicker(20 * time.Second)
		defer ping.Stop()
		for {
			select {
			case b := <-ch:
				fmt.Fprintf(w, "data: %s\n\n", b)
				fl.Flush()
			case <-ping.C:
				fmt.Fprint(w, ": ping\n\n")
				fl.Flush()
			case <-r.Context().Done():
				return true
			}
		}
	case "/api/view/open":
		t, err := browser.ViewTab(root, id, int(num("width")), int(num("height")))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		t.Resize(int(num("width")), int(num("height")), num("dpr"))
		if u := str("url"); u != "" {
			navErr, err := t.Navigate(r.Context(), u)
			if err != nil || navErr != "" {
				sendJSON(w, 200, map[string]any{"ok": false, "error": core.FirstNonEmpty(navErr, errString(err))})
				return true
			}
		}
		sendJSON(w, 200, map[string]any{"ok": true})
	case "/api/view/input":
		t, err := browser.ViewTab(root, id, 0, 0)
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		pick, err := t.Input(r.Context(), str("kind"), body)
		if str("kind") == "pick" {
			if err != nil || pick == nil {
				sendJSON(w, 200, map[string]any{"ok": false, "error": core.FirstNonEmpty(errString(err), "nada nesse ponto")})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "pick": pick})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
	default:
		return false
	}
	return true
}

func closeViewBrowsers() { browser.CloseViewTabs() }
