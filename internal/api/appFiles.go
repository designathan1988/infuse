package api

import (
	"infuse/internal/agent"
	"infuse/internal/desktop"
	"infuse/internal/workspace"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// appFiles handles one group of application routes.

func appFiles(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- files and other apps
	case path == "/api/file" && r.Method == http.MethodGet:
		fileReadRoute(w, q, dir)
		return true
	case path == "/api/file" && r.Method == http.MethodPut:
		fileWriteRoute(w, body, dir, str, boolean)
		return true
	case path == "/api/editors":
		sendJSON(w, 200, workspace.Editors())
		return true
	case path == "/api/frameable":
		sendJSON(w, 200, map[string]any{"frameable": agent.Frameable(r.Context(), q["url"])})
		return true
	case path == "/api/websearch":
		webSearchRoute(w, r, q)
		return true
	case path == "/api/open" && str("url") != "":
		openURLRoute(w, str)
		return true
	case path == "/api/open":
		openPathRoute(w, dir, str)
		return true
	}
	return false
}

func fileReadRoute(w http.ResponseWriter, q map[string]string, dir string) {
	p := filepath.Join(dir, filepath.FromSlash(q["path"]))
	if rel, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(rel, "..") {
		sendJSON(w, 400, map[string]any{"error": "fora do projeto"})
		return
	}
	st, err := os.Stat(p)
	if err != nil {
		sendJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	b, _ := os.ReadFile(p)
	binary := strings.ContainsRune(string(b[:min(len(b), 8000)]), 0)
	if binary || len(b) > 2<<20 {
		sendJSON(w, 200, map[string]any{"path": q["path"], "binary": true, "size": st.Size(), "mtime": st.ModTime().UnixMilli()})
		return
	}
	sendJSON(w, 200, map[string]any{"path": q["path"], "content": string(b), "mtime": st.ModTime().UnixMilli(), "abs": p,
		"readonly": strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/journal") || strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/sessions")})
}

func fileWriteRoute(w http.ResponseWriter, body map[string]any, dir string, str func(string) string, boolean func(string) bool) {
	mtime, _ := body["mtime"].(float64)
	nm, err := workspace.WriteUserFile(dir, str("path"), str("content"), int64(mtime), boolean("force"))
	if err == workspace.ErrConflict {
		cur, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(str("path"))))
		sendJSON(w, 409, map[string]any{"conflict": true, "mtime": nm, "content": string(cur)})
		return
	}
	if err != nil {
		sendJSON(w, 400, map[string]any{"error": err.Error()})
		return
	}
	sendJSON(w, 200, map[string]any{"ok": true, "mtime": nm})
}

func webSearchRoute(w http.ResponseWriter, r *http.Request, q map[string]string) {
	res, engine, err := agent.WebSearch(r.Context(), q["q"], 12)
	if err != nil {
		sendJSON(w, 200, map[string]any{"error": err.Error(), "results": []agent.SearchResult{}})
		return
	}
	sendJSON(w, 200, map[string]any{"results": res, "engine": engine})
}

func openURLRoute(w http.ResponseWriter, str func(string) string) {
	// "open in the default browser", asked by the user from the browser pane
	u := str("url")
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		sendJSON(w, 400, map[string]any{"error": "endereço inválido"})
		return
	}
	sendJSON(w, 200, map[string]any{"ok": agent.OpenDefaultBrowser(u) == nil})
}

func openPathRoute(w http.ResponseWriter, dir string, str func(string) string) {
	p := str("path")
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, filepath.FromSlash(p))
	}
	var err error
	if str("app") == "explorer" {
		err = desktop.RevealInFileManager(p)
	} else {
		err = workspace.OpenInEditor(str("app"), p)
	}
	res := map[string]any{"ok": err == nil}
	if err != nil {
		res["error"] = err.Error()
	}
	sendJSON(w, 200, res)
}
