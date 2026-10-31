package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// studioRoutes2 handles one group of studio routes.

func studioRoutes2(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root string, str func(string) string) bool {
	path := r.URL.Path
	switch {
	case path == "/api/commands":
		sendJSON(w, 200, listCommands(root))
		return true
	case path == "/api/terminal":
		// The integrated terminal: the same interposed shell the agent uses, visible. One command, its output.
		sendJSON(w, 200, runTerminal(root, str("cwd"), str("command")))
		return true
	case path == "/api/files/tree":
		sendJSON(w, 200, fileTree(root, root, 0))
		return true
	case path == "/api/files/read":
		p := filepath.Join(root, filepath.FromSlash(q["path"]))
		if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
			sendJSON(w, 400, map[string]any{"error": "fora do projeto"})
			return true
		}
		b, err := os.ReadFile(p)
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": err.Error()})
			return true
		}
		if len(b) > 1<<20 {
			b = b[:1<<20]
		}
		sendJSON(w, 200, map[string]any{"path": q["path"], "content": string(b)})
		return true
	case path == "/api/diff":
		if !exists(filepath.Join(root, ".git")) {
			sendJSON(w, 200, map[string]any{"diff": LocalDiff(firstNonEmpty(q["project"], root), root, q["session"], q["scope"], q["path"])})
			return true
		}
		sendJSON(w, 200, gitDiff(root, q["path"]))
		return true
	case path == "/api/git":
		action := str("action")
		switch action {
		case "commit":
			msg := str("message")
			if strings.TrimSpace(msg) == "" {
				sendJSON(w, 400, map[string]any{"error": "mensagem vazia"})
				return true
			}
			out, err := gitRun(root, "add", "-A")
			if err == nil {
				out, err = gitRun(root, "commit", "-m", msg)
			}
			sendJSON(w, 200, map[string]any{"ok": err == nil, "output": out})
		case "discard":
			if !exists(filepath.Join(root, ".git")) { // a local folder: back to the copy before the first change
				err := LocalDiscard(firstNonEmpty(q["project"], root), root, q["session"], q["scope"], str("path"))
				sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
				return true
			}
			out, err := gitRun(root, "checkout", "--", str("path"))
			sendJSON(w, 200, map[string]any{"ok": err == nil, "output": out})
		default:
			sendJSON(w, 200, map[string]any{"branch": gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD"), "status": gitOutput(root, "status", "--porcelain"), "log": gitOutput(root, "log", "--oneline", "-15")})
		}
		return true
	case path == "/api/oracle":
		dir := filepath.Join(root, infuseDirName, "oracle")
		if r.Method == http.MethodPut {
			_ = os.MkdirAll(dir, 0o755)
			name := reSafeName.ReplaceAllString(str("name"), "_")
			if name == "" {
				name = "pronto.md"
			}
			_ = os.WriteFile(filepath.Join(dir, name), []byte(str("content")), 0o644)
			sendJSON(w, 200, map[string]any{"ok": true})
			return true
		}
		entries, _ := os.ReadDir(dir)
		files := []map[string]string{}
		for _, e := range entries {
			if !e.IsDir() {
				files = append(files, map[string]string{"name": e.Name(), "content": readIfExists(filepath.Join(dir, e.Name()))})
			}
		}
		sendJSON(w, 200, files)
		return true
	case path == "/api/memory":
		file := filepath.Join(root, infuseDirName, "memory.md")
		if r.Method == http.MethodPut {
			_ = os.MkdirAll(filepath.Dir(file), 0o755)
			_ = os.WriteFile(file, []byte(str("content")), 0o644)
			sendJSON(w, 200, map[string]any{"ok": true})
			return true
		}
		sendJSON(w, 200, map[string]any{"content": readIfExists(file), "instructions": loadInstructions(root)})
		return true
	}
	return false
}
