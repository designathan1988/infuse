package api

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// appHooks handles one group of application routes.

func appHooks(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	// ---------------------------------------------------------------- hooks and skills
	case path == "/api/hooks" && r.Method == http.MethodGet:
		out := LoadHooks(root)
		if out == nil {
			out = []HookEntry{}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/hooks/add":
		ev, cmdline := str("event"), strings.TrimSpace(str("command"))
		valid := false
		for _, e := range hookEvents {
			valid = valid || e == ev
		}
		if !valid || cmdline == "" {
			sendJSON(w, 200, map[string]any{"ok": false, "error": "escolha o momento e escreva o comando"})
			return true
		}
		scope := str("scope")
		if scope != "global" {
			scope = "project"
		}
		err := addHook(root, scope, ev, str("matcher"), cmdline)
		if err == nil { // added by the user in the app: approved in the same click
			err = approveHook(hookKey(root, scope, ev, str("matcher"), cmdline))
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/hooks/approve":
		sendJSON(w, 200, map[string]any{"ok": approveHook(str("key")) == nil})
		return true
	case path == "/api/hooks/remove":
		err := removeHook(root, str("key"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/skills" && r.Method == http.MethodGet:
		out := LoadSkills(root)
		for i := range out {
			if b, err := os.ReadFile(filepath.Join(out[i].Dir, "SKILL.md")); err == nil {
				_, _, out[i].Body = parseSkill(string(b))
			}
		}
		if out == nil {
			out = []Skill{}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/skills/save":
		name := strings.TrimSpace(str("name"))
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(name) {
			sendJSON(w, 200, map[string]any{"ok": false, "error": "nome: letras minúsculas, números e hífens (ex.: revisar-pr)"})
			return true
		}
		dir := filepath.Join(root, infuseDirName, "skills", name)
		if str("scope") == "global" {
			dir = filepath.Join(InfuseHome(), "skills", name)
		}
		desc := strings.ReplaceAll(strings.TrimSpace(str("description")), "\n", " ")
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + strings.TrimSpace(str("body")) + "\n"
		_ = os.MkdirAll(dir, 0o755)
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/skills/remove":
		for _, s := range LoadSkills(root) {
			if s.Name == str("name") && (strings.HasPrefix(s.Dir, filepath.Join(root, infuseDirName)) || strings.HasPrefix(s.Dir, InfuseHome())) {
				_ = os.RemoveAll(s.Dir)
			}
		}
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	}
	return false
}
