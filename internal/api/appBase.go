package api

// The architecture base: read the current choice and the starter catalog, apply a starter or a custom
// base, scaffold one in the project, or scaffold a new project from one. A scaffolded project gets its
// base document and a generated map immediately.

import (
	"infuse/internal/base"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/projectmap"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reBaseProjectName = regexp.MustCompile(`^[A-Za-z0-9 _.-]+$`)

func baseRoute(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string) bool {
	switch r.URL.Path {
	case "/api/base":
		if r.Method == http.MethodGet {
			if root == "" {
				sendJSON(w, http.StatusBadRequest, map[string]any{"error": "project required"})
				return true
			}
			sendJSON(w, http.StatusOK, baseView(root))
			return true
		}
		if r.Method == http.MethodPut {
			if root == "" {
				sendJSON(w, http.StatusBadRequest, map[string]any{"error": "project required"})
				return true
			}
			if err := saveBase(root, body); err != nil {
				sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "base.failed"})
				return true
			}
			projectmap.WriteMap(root, projectmap.BuildMap(root))
			sendJSON(w, http.StatusOK, baseView(root))
			return true
		}
	case "/api/base/scaffold":
		scaffoldRoute(w, r, root, str)
		return true
	}
	return false
}

// scaffoldRoute creates a base in the open project, or a new project from one in a folder the user chose.
func scaffoldRoute(w http.ResponseWriter, r *http.Request, root string, str func(string) string) {
	if r.Method != http.MethodPost {
		sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	parent, name, starter := str("parent"), strings.TrimSpace(str("name")), str("base")
	if parent == "" && root != "" { // the open project itself
		b, err := base.Scaffold(root, starter)
		if err != nil {
			sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "base.failed"})
			return
		}
		sendJSON(w, http.StatusOK, map[string]any{"ok": true, "base": b})
		return
	}
	if parent == "" || !filepath.IsAbs(parent) || name == "" || name == "." || name == ".." ||
		filepath.Base(name) != name || !reBaseProjectName.MatchString(name) || starter == "" {
		sendJSON(w, http.StatusBadRequest, map[string]any{"error": "parent, name and base required", "code": "base.invalidName"})
		return
	}
	if st, err := os.Stat(parent); err != nil || !st.IsDir() {
		sendJSON(w, http.StatusBadRequest, map[string]any{"error": "parent folder does not exist", "code": "base.invalidName"})
		return
	}
	target := filepath.Join(parent, name)
	if err := os.MkdirAll(target, 0o755); err != nil {
		sendJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	b, err := base.Scaffold(target, starter)
	if err != nil {
		sendJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "code": "base.failed"})
		return
	}
	projectmap.WriteMap(target, projectmap.BuildMap(target))
	config.RememberProject(target)
	sendJSON(w, http.StatusOK, map[string]any{"ok": true, "root": target, "base": b.Name, "summary": b.Summary(), "file": filepath.Join(target, core.InfuseDirName, "base.json")})
}

func baseView(root string) map[string]any {
	file := filepath.Join(root, core.InfuseDirName, "base.json")
	_, statErr := os.Stat(file)
	current := base.Load(root)
	view := map[string]any{
		"ok":      true,
		"catalog": base.Catalog(),
		"current": current,
		"file":    file,
		"exists":  statErr == nil,
	}
	if b := current; b != nil {
		view["name"] = b.Name
		view["summary"] = b.Summary()
		view["folders"] = b.Folders
	}
	return view
}
