package api

import (
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/workspace"
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
		studioTerminal(w, root, str)
		return true
	case path == "/api/files/tree":
		sendJSON(w, 200, fileTree(root, root, 0))
		return true
	case path == "/api/files/read":
		studioFileRead(w, q, root)
		return true
	case path == "/api/diff":
		studioDiff(w, q, root)
		return true
	case path == "/api/git":
		studioGit(w, q, root, str)
		return true
	case path == "/api/oracle":
		studioOracle(w, r, root, str)
		return true
	case path == "/api/memory":
		studioMemory(w, r, root, str)
		return true
	}
	return false
}

func studioTerminal(w http.ResponseWriter, root string, str func(string) string) {
	// The integrated terminal: the same interposed shell the agent uses, visible. One command, its output.
	sendJSON(w, 200, runTerminal(root, str("cwd"), str("command")))
}

func studioFileRead(w http.ResponseWriter, q map[string]string, root string) {
	p := filepath.Join(root, filepath.FromSlash(q["path"]))
	if rel, err := filepath.Rel(root, p); err != nil || strings.HasPrefix(rel, "..") {
		sendJSON(w, 400, map[string]any{"error": "fora do projeto"})
		return
	}
	b, err := os.ReadFile(p)
	if err != nil {
		sendJSON(w, 404, map[string]any{"error": err.Error()})
		return
	}
	if len(b) > 1<<20 {
		b = b[:1<<20]
	}
	sendJSON(w, 200, map[string]any{"path": q["path"], "content": string(b)})
}

func studioDiff(w http.ResponseWriter, q map[string]string, root string) {
	if !core.Exists(filepath.Join(root, ".git")) {
		sendJSON(w, 200, map[string]any{"diff": workspace.LocalDiff(core.FirstNonEmpty(q["project"], root), root, q["session"], q["scope"], q["path"], snapshots{})})
		return
	}
	sendJSON(w, 200, gitDiff(root, q["path"]))
}

func studioGit(w http.ResponseWriter, q map[string]string, root string, str func(string) string) {
	action := str("action")
	switch action {
	case "commit":
		msg := str("message")
		if strings.TrimSpace(msg) == "" {
			sendJSON(w, 400, map[string]any{"error": "mensagem vazia"})
			return
		}
		out, err := workspace.GitRun(root, "add", "-A")
		if err == nil {
			out, err = workspace.GitRun(root, "commit", "-m", msg)
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "output": out})
	case "discard":
		if !core.Exists(filepath.Join(root, ".git")) { // a local folder: back to the copy before the first change
			err := workspace.LocalDiscard(core.FirstNonEmpty(q["project"], root), root, q["session"], q["scope"], str("path"), snapshots{})
			sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
			return
		}
		out, err := workspace.GitRun(root, "checkout", "--", str("path"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "output": out})
	default:
		sendJSON(w, 200, map[string]any{"branch": workspace.GitOutput(root, "rev-parse", "--abbrev-ref", "HEAD"), "status": workspace.GitOutput(root, "status", "--porcelain"), "log": workspace.GitOutput(root, "log", "--oneline", "-15")})
	}
}

func studioOracle(w http.ResponseWriter, r *http.Request, root string, str func(string) string) {
	dir := filepath.Join(root, core.InfuseDirName, "oracle")
	if r.Method == http.MethodPut {
		_ = os.MkdirAll(dir, 0o755)
		name := reSafeName.ReplaceAllString(str("name"), "_")
		if name == "" {
			name = "pronto.md"
		}
		_ = os.WriteFile(filepath.Join(dir, name), []byte(str("content")), 0o644)
		sendJSON(w, 200, map[string]any{"ok": true})
		return
	}
	entries, _ := os.ReadDir(dir)
	files := []map[string]string{}
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, map[string]string{"name": e.Name(), "content": agent.ReadIfExists(filepath.Join(dir, e.Name()))})
		}
	}
	sendJSON(w, 200, files)
}

func studioMemory(w http.ResponseWriter, r *http.Request, root string, str func(string) string) {
	file := filepath.Join(root, core.InfuseDirName, "memory.md")
	if r.Method == http.MethodPut {
		_ = os.MkdirAll(filepath.Dir(file), 0o755)
		_ = os.WriteFile(file, []byte(str("content")), 0o644)
		sendJSON(w, 200, map[string]any{"ok": true})
		return
	}
	sendJSON(w, 200, map[string]any{"content": agent.ReadIfExists(file), "instructions": agent.LoadInstructions(root)})
}
