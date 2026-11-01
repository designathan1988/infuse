package api

import (
	"infuse/internal/core"
	"infuse/internal/terminal"
	"net/http"
	"strings"
)

// appTerminal handles one group of application routes.

func appTerminal(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- terminal
	case path == "/api/term/open":
		base := core.FirstNonEmpty(root, dir)
		cwd, err := terminal.ResolveDir(base, str("cwd"))
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": "no directory to start the terminal in", "code": "terminal.noDir"})
			return true
		}
		t, err := OpenTerminal(core.FirstNonEmpty(root, cwd), cwd, num("cols"), num("rows"))
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error(), "code": "terminal.openFailed", "args": map[string]any{"error": err.Error()}})
			return true
		}
		if run := str("run"); run != "" { // e.g. an agent CLI: started by the shell, so it runs in the project folder
			_, _ = t.Write([]byte(run + "\r"))
		}
		sendJSON(w, 200, map[string]any{"id": t.ID, "cwd": t.Cwd})
		return true
	case strings.HasPrefix(path, "/api/term/"):
		parts := strings.SplitN(strings.TrimPrefix(path, "/api/term/"), "/", 2)
		t := getTerminal(parts[0])
		if t == nil || len(parts) < 2 {
			sendJSON(w, 404, map[string]any{"error": "the terminal has ended", "code": "terminal.gone"})
			return true
		}
		switch parts[1] {
		case "stream":
			streamTerminal(w, r, t)
		case "input":
			_, _ = t.Write([]byte(str("data")))
			sendJSON(w, 200, map[string]any{"ok": true})
		case "resize":
			_ = t.Resize(num("cols"), num("rows"))
			sendJSON(w, 200, map[string]any{"ok": true})
		case "close":
			CloseTerminal(t.ID)
			sendJSON(w, 200, map[string]any{"ok": true})
		default:
			return false
		}
		return true
	}
	return false
}
