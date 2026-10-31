package main

// Studio API: sessions, streaming events (SSE), files, diff, settings.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

func studioAPI(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any) bool {
	root := q["root"]
	str := func(k string) string { s, _ := body[k].(string); return s }
	path := r.URL.Path

	switch {
	case path == "/api/settings":
		if r.Method == http.MethodPut {
			s := LoadSettings()
			b, _ := json.Marshal(body)
			if err := json.Unmarshal(b, &s); err != nil {
				sendJSON(w, 400, map[string]any{"error": err.Error()})
				return true
			}
			if err := SaveSettings(s); err != nil {
				sendJSON(w, 500, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true})
			return true
		}
		s := LoadSettings()
		masked := s
		if len(masked.APIKey) > 8 {
			masked.APIKey = masked.APIKey[:6] + "…" + masked.APIKey[len(masked.APIKey)-4:]
		}
		sendJSON(w, 200, map[string]any{"settings": masked, "hasKey": s.APIKey != "", "providers": providerNames(), "tools": toolSpecs()})
		return true

	case path == "/api/settings/test":
		s := LoadSettings()
		if str("apiKey") != "" && !strings.Contains(str("apiKey"), "…") {
			s.APIKey = str("apiKey")
		}
		p, err := newProvider(s.Provider, ProviderConfig{APIKey: s.APIKey, BaseURL: s.BaseURL})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		res, err := p.Stream(r.Context(), ModelRequest{Model: s.Model, Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: "Responda apenas: ok"}}}}, MaxTokens: 16}, func(StreamEvent) {})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "model": s.Model, "usage": res.Usage})
		return true

	case path == "/api/sessions" && r.Method == http.MethodGet:
		sendJSON(w, 200, listSessions(root))
		return true
	case path == "/api/sessions" && r.Method == http.MethodPost:
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "projeto não encontrado"})
			return true
		}
		s := newSession(root)
		sendJSON(w, 200, s.snapshot())
		return true

	case strings.HasPrefix(path, "/api/sessions/"):
		rest := strings.TrimPrefix(path, "/api/sessions/")
		parts := strings.SplitN(rest, "/", 2)
		id := parts[0]
		action := ""
		if len(parts) > 1 {
			action = parts[1]
		}
		s, err := loadSession(root, id)
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
			return true
		}
		switch action {
		case "":
			switch r.Method {
			case http.MethodDelete:
				s.Stop()
				sessionsMu.Lock()
				delete(sessions, id)
				sessionsMu.Unlock()
				_ = os.Remove(filepath.Join(sessionsDir(root), id+".json"))
				_ = os.RemoveAll(filepath.Join(root, infuseDirName, "checkpoints", id))
				sendJSON(w, 200, map[string]any{"ok": true})
			case http.MethodPut:
				s.mu.Lock()
				if t := str("title"); t != "" {
					s.Title = t
				}
				if m := str("model"); m != "" {
					s.Model = m
				}
				s.mu.Unlock()
				s.save()
				sendJSON(w, 200, s.snapshot())
			default:
				sendJSON(w, 200, s.snapshot())
			}
		case "duplicate":
			d := newSession(root)
			d.mu.Lock()
			d.Title = s.Title + " (cópia)"
			d.Model = s.Model
			d.Messages = s.copyMessages()
			d.Turn = s.Turn
			d.TurnStarts = append([]int{}, s.TurnStarts...)
			d.mu.Unlock()
			d.save()
			sendJSON(w, 200, d.snapshot())
		case "send":
			var atts []Attachment
			ab, _ := json.Marshal(body["attachments"])
			_ = json.Unmarshal(ab, &atts)
			var imgs []InlineImage
			ib, _ := json.Marshal(body["images"])
			_ = json.Unmarshal(ib, &imgs)
			if err := s.Send(str("text"), atts, imgs); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "turn": s.Turn})
		case "stop":
			s.Stop()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "permission":
			sendJSON(w, 200, map[string]any{"ok": s.ResolvePermission(str("id"), str("decision"))})
		case "answer":
			sendJSON(w, 200, map[string]any{"ok": s.Answer(str("id"), str("answer"))})
		case "revert":
			t, _ := strconv.Atoi(fmt.Sprint(body["turn"]))
			if err := s.Revert(t); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, s.snapshot())
		case "events":
			streamEvents(w, r, s)
		default:
			http.NotFound(w, r)
		}
		return true

	case path == "/api/files/list":
		// Flat list for the @ autocomplete: every file and directory the map knows, project-relative.
		m := LoadOrBuildMap(root)
		out := []string{}
		seen := map[string]bool{}
		for p := range m.Files {
			out = append(out, p)
			for d := filepath.ToSlash(filepath.Dir(p)); d != "." && d != "/" && !seen[d]; d = filepath.ToSlash(filepath.Dir(d)) {
				seen[d] = true
				out = append(out, d+"/")
			}
		}
		sort.Strings(out)
		sendJSON(w, 200, out)
		return true
	case path == "/api/apps":
		sendJSON(w, 200, DetectApps())
		return true
	case path == "/api/apps/open":
		res, err := LaunchApp(str("id"), root)
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "launched": res})
		return true
	case path == "/api/providers/models":
		s := LoadSettings()
		name := str("provider")
		if name == "" {
			name = s.Provider
		}
		key := s.APIKey
		if k := str("apiKey"); k != "" && !strings.Contains(k, "…") {
			key = k
		}
		base := s.BaseURL
		if b, ok := body["baseUrl"].(string); ok {
			base = b
		}
		names, err := ListModels(r.Context(), name, ProviderConfig{APIKey: key, BaseURL: base})
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "models": names})
		return true
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

func providerNames() []string {
	var out []string
	for k := range providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func streamEvents(w http.ResponseWriter, r *http.Request, s *Session) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", 500)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	ch, unsub := s.subscribe()
	defer unsub()
	write := func(ev Event) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	}
	write(Event{Type: "session", Data: s.snapshot()})
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return // dropped for falling behind: the browser reconnects and gets a snapshot
			}
			write(ev)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

type treeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	Dir      bool        `json:"dir"`
	Children []*treeNode `json:"children,omitempty"`
	Lines    int         `json:"lines,omitempty"`
}

func fileTree(root, dir string, depth int) *treeNode {
	rel := relTo(root, dir)
	n := &treeNode{Name: filepath.Base(dir), Path: rel, Dir: true}
	if rel == "" {
		n.Name = filepath.Base(root)
	}
	if depth > 12 {
		return n
	}
	entries, _ := os.ReadDir(dir)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	m := LoadMap(root)
	for _, e := range entries {
		if e.IsDir() {
			if ignoredDirs[e.Name()] || (strings.HasPrefix(e.Name(), ".") && e.Name() != ".infuse") {
				continue
			}
			if e.Name() == ".infuse" {
				continue
			}
			n.Children = append(n.Children, fileTree(root, filepath.Join(dir, e.Name()), depth+1))
			continue
		}
		if e.Name() == ".git" { // a worktree's .git is a file pointing at the main repository
			continue
		}
		childRel := e.Name()
		if rel != "" {
			childRel = rel + "/" + e.Name()
		}
		c := &treeNode{Name: e.Name(), Path: childRel}
		if m != nil {
			if f, ok := m.Files[childRel]; ok {
				c.Lines = f.Lines
			}
		}
		n.Children = append(n.Children, c)
	}
	return n
}

func gitRun(root string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	noConsoleWindow(cmd)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func gitDiff(root, path string) map[string]any {
	if path != "" {
		out, _ := gitRun(root, "diff", "--", path)
		if strings.TrimSpace(out) == "" {
			// untracked: show whole file as added
			if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err == nil {
				lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
				for i := range lines {
					lines[i] = "+" + lines[i]
				}
				out = "--- /dev/null\n+++ b/" + path + "\n" + strings.Join(lines, "\n")
			}
		}
		return map[string]any{"path": path, "diff": out}
	}
	status, _ := gitRun(root, "status", "--porcelain") // untrimmed: the first column is significant
	files := []map[string]string{}
	for _, l := range strings.Split(status, "\n") {
		if len(l) < 4 {
			continue
		}
		files = append(files, map[string]string{"status": strings.TrimSpace(l[:2]), "path": strings.TrimSpace(l[3:])})
	}
	stat, _ := gitRun(root, "diff", "--stat")
	return map[string]any{"files": files, "stat": stat, "isGit": exists(filepath.Join(root, ".git"))}
}
