package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// appCommands handles one group of application routes.

func appCommands(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- custom commands and instructions
	case path == "/api/commands/save":
		name := taskSlug(str("name"))
		if name == "" {
			sendJSON(w, 400, map[string]any{"error": "nome inválido"})
			return true
		}
		base := filepath.Join(root, infuseDirName, "commands")
		if str("scope") == "global" {
			base = filepath.Join(InfuseHome(), "commands")
		}
		_ = os.MkdirAll(base, 0o755)
		content := str("prompt")
		if d := strings.TrimSpace(str("description")); d != "" {
			content = "---\ndescription: " + d + "\n---\n" + content
		}
		if err := os.WriteFile(filepath.Join(base, name+".md"), []byte(content), 0o644); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		if old := taskSlug(str("previousName")); old != "" && old != name {
			_ = os.Remove(filepath.Join(base, old+".md"))
		}
		sendJSON(w, 200, listCommands(root))
		return true
	case path == "/api/commands/delete":
		for _, base := range []string{filepath.Join(root, infuseDirName, "commands"), filepath.Join(InfuseHome(), "commands")} {
			_ = os.Remove(filepath.Join(base, taskSlug(str("name"))+".md"))
		}
		sendJSON(w, 200, listCommands(root))
		return true
	case path == "/api/projectmode":
		sendJSON(w, 200, map[string]any{"mode": projectMode(root)})
		return true
	case path == "/api/settings/full":
		// settings without secrets plus what the settings screen needs
		s := LoadSettings()
		s.APIKey = ""
		sendJSON(w, 200, map[string]any{"settings": s, "version": version, "shell": GetStatus()})
		return true
	}
	return false
}
