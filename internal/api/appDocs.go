package api

import (
	"infuse/internal/agent"
	"infuse/internal/knowledge"
	"net/http"
	"os"
	"path/filepath"
)

// appDocs handles one group of application routes.

func appDocs(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- pinned docs and conventions
	case path == "/api/docs" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"docs": agent.LoadDocs(root), "suggestions": agent.SuggestDocs(root)})
		return true
	case path == "/api/docs/pin":
		doc, err := agent.PinDoc(r.Context(), root, str("url"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "doc": doc})
		return true
	case path == "/api/docs/unpin":
		err := agent.UnpinDoc(root, str("url"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/conventions" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"text": knowledge.ConventionsPrompt(root), "file": knowledge.ConventionsFile(root)})
		return true
	case path == "/api/conventions":
		var err error
		if boolean("relearn") {
			text := knowledge.LearnConventions(root)
			_ = os.MkdirAll(filepath.Dir(knowledge.ConventionsFile(root)), 0o755)
			err = os.WriteFile(knowledge.ConventionsFile(root), []byte(text), 0o644)
		} else {
			err = os.WriteFile(knowledge.ConventionsFile(root), []byte(str("text")), 0o644)
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "text": knowledge.ConventionsPrompt(root)})
		return true
	}
	return false
}
