package api

// Routes for the desktop-style window: providers, models, sessions (create with worktree, archive,
// rewind variants, side chat, context), review (changes, PR), files, editors, terminal, dev servers,
// scheduled tasks, usage totals.

import (
	"infuse/internal/config"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
)

func appAPI(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any) bool {
	root := q["root"]
	path := r.URL.Path
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) int { f, _ := body[k].(float64); return int(f) }
	boolean := func(k string) bool { b, _ := body[k].(bool); return b }
	if strings.HasPrefix(path, "/api/view/") && viewAPI(w, r, root, path, body) { // the streamed browser pane
		return true
	}
	// dir: where git and file operations run (a session's worktree), always inside the project.
	dir := root
	if d := q["dir"]; d != "" {
		if rel, err := filepath.Rel(root, d); err == nil && !strings.HasPrefix(rel, "..") {
			dir = d
		}
	}

	for _, group := range appGroups {
		if group(w, r, q, body, root, dir, str, num, boolean) {
			return true
		}
	}
	return false
}

// rememberProjectMode: the mode chosen in the selector is remembered per folder for new sessions.
func rememberProjectMode(root, mode string) {
	if mode == "" {
		return
	}
	list := config.LoadProjects()
	key := config.RootKey(root)
	for i := range list {
		if config.RootKey(list[i].Root) == key {
			list[i].Mode = mode
		}
	}
	_ = config.SaveProjects(list)
}

func projectMode(root string) string {
	key := config.RootKey(root)
	for _, p := range config.LoadProjects() {
		if config.RootKey(p.Root) == key {
			return p.Mode
		}
	}
	return ""
}

var _ = strconv.Itoa
