package api

// Studio API: sessions, streaming events (SSE), files, diff, settings.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/projectmap"
	"infuse/internal/workspace"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func studioAPI(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any) bool {
	root := q["root"]
	str := func(k string) string { s, _ := body[k].(string); return s }

	for _, group := range studioGroups {
		if group(w, r, q, body, root, str) {
			return true
		}
	}
	return false
}

func streamEvents(w http.ResponseWriter, r *http.Request, s *agent.Session,

) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", 500)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	w.Header().Set("connection", "keep-alive")
	ch, unsub := s.Subscribe()
	defer unsub()
	write := func(ev core.Event,

	) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		fl.Flush()
	}
	write(core.Event{Type: "session", Data: s.Snapshot()})
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
	rel := core.RelTo(root, dir)
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
	m := projectmap.LoadMap(root)
	for _, e := range entries {
		if e.IsDir() {
			if projectmap.IgnoredDirs[e.Name()] || (strings.HasPrefix(e.Name(), ".") && e.Name() != ".infuse") {
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

func gitDiff(root, path string) map[string]any {
	if path != "" {
		out, _ := workspace.GitRun(root, "diff", "--", path)
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
	status, _ := workspace.GitRun(root, "status", "--porcelain") // untrimmed: the first column is significant
	files := []map[string]string{}
	for _, l := range strings.Split(status, "\n") {
		if len(l) < 4 {
			continue
		}
		files = append(files, map[string]string{"status": strings.TrimSpace(l[:2]), "path": strings.TrimSpace(l[3:])})
	}
	stat, _ := workspace.GitRun(root, "diff", "--stat")
	return map[string]any{"files": files, "stat": stat, "isGit": core.Exists(filepath.Join(root, ".git"))}
}
