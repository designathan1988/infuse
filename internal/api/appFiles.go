package api

import (
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
		p := filepath.Join(dir, filepath.FromSlash(q["path"]))
		if rel, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(rel, "..") {
			sendJSON(w, 400, map[string]any{"error": "fora do projeto"})
			return true
		}
		st, err := os.Stat(p)
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": err.Error()})
			return true
		}
		b, _ := os.ReadFile(p)
		binary := strings.ContainsRune(string(b[:min(len(b), 8000)]), 0)
		if binary || len(b) > 2<<20 {
			sendJSON(w, 200, map[string]any{"path": q["path"], "binary": true, "size": st.Size(), "mtime": st.ModTime().UnixMilli()})
			return true
		}
		sendJSON(w, 200, map[string]any{"path": q["path"], "content": string(b), "mtime": st.ModTime().UnixMilli(), "abs": p,
			"readonly": strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/journal") || strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/sessions")})
		return true
	case path == "/api/file" && r.Method == http.MethodPut:
		mtime, _ := body["mtime"].(float64)
		nm, err := WriteUserFile(dir, str("path"), str("content"), int64(mtime), boolean("force"))
		if err == errConflict {
			cur, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(str("path"))))
			sendJSON(w, 409, map[string]any{"conflict": true, "mtime": nm, "content": string(cur)})
			return true
		}
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "mtime": nm})
		return true
	case path == "/api/editors":
		sendJSON(w, 200, Editors())
		return true
	case path == "/api/frameable":
		sendJSON(w, 200, map[string]any{"frameable": frameable(r.Context(), q["url"])})
		return true
	case path == "/api/websearch":
		res, engine, err := WebSearch(r.Context(), q["q"], 12)
		if err != nil {
			sendJSON(w, 200, map[string]any{"error": err.Error(), "results": []SearchResult{}})
			return true
		}
		sendJSON(w, 200, map[string]any{"results": res, "engine": engine})
		return true
	case path == "/api/open" && str("url") != "":
		// "open in the default browser", asked by the user from the browser pane
		u := str("url")
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			sendJSON(w, 400, map[string]any{"error": "endereço inválido"})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": openDefaultBrowser(u) == nil})
		return true
	case path == "/api/open":
		p := str("path")
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, filepath.FromSlash(p))
		}
		var err error
		if str("app") == "explorer" {
			err = revealInFileManager(p)
		} else {
			err = OpenInEditor(str("app"), p)
		}
		res := map[string]any{"ok": err == nil}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true
	}
	return false
}
