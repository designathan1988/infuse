package api

import (
	"infuse/internal/agent"
	"infuse/internal/core"
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
		hooksList(w, root)
		return true
	case path == "/api/hooks/add":
		hooksAdd(w, root, str)
		return true
	case path == "/api/hooks/approve":
		sendJSON(w, 200, map[string]any{"ok": agent.ApproveHook(str("key")) == nil})
		return true
	case path == "/api/hooks/remove":
		err := agent.RemoveHook(root, str("key"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/skills" && r.Method == http.MethodGet:
		skillsList(w, root)
		return true
	case path == "/api/skills/save":
		skillsSave(w, root, str)
		return true
	case path == "/api/skills/remove":
		skillsRemove(w, root, str)
		return true
	}
	return false
}

func hooksList(w http.ResponseWriter, root string) {
	out := agent.LoadHooks(root)
	if out == nil {
		out = []agent.HookEntry{}
	}
	sendJSON(w, 200, out)
}

func hooksAdd(w http.ResponseWriter, root string, str func(string) string) {
	ev, cmdline := str("event"), strings.TrimSpace(str("command"))
	valid := false
	for _, e := range agent.HookEvents {
		valid = valid || e == ev
	}
	if !valid || cmdline == "" {
		sendJSON(w, 200, map[string]any{"ok": false, "error": "escolha o momento e escreva o comando"})
		return
	}
	scope := str("scope")
	if scope != "global" {
		scope = "project"
	}
	err := agent.AddHook(root, scope, ev, str("matcher"), cmdline)
	if err == nil { // added by the user in the app: approved in the same click
		err = agent.ApproveHook(agent.HookKey(root, scope, ev, str("matcher"), cmdline))
	}
	sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
}

func skillsList(w http.ResponseWriter, root string) {
	out := agent.LoadSkills(root)
	for i := range out {
		if b, err := os.ReadFile(filepath.Join(out[i].Dir, "SKILL.md")); err == nil {
			_, _, out[i].Body = agent.ParseSkill(string(b))
		}
	}
	if out == nil {
		out = []agent.Skill{}
	}
	sendJSON(w, 200, out)
}

func skillsSave(w http.ResponseWriter, root string, str func(string) string) {
	name := strings.TrimSpace(str("name"))
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(name) {
		sendJSON(w, 200, map[string]any{"ok": false, "error": "name: lowercase letters, numbers and hyphens (e.g. review-pr)", "code": "skill.invalidName"})
		return
	}
	dir := filepath.Join(root, core.InfuseDirName, "skills", name)
	if str("scope") == "global" {
		dir = filepath.Join(core.InfuseHome(), "skills", name)
	}
	desc := strings.ReplaceAll(strings.TrimSpace(str("description")), "\n", " ")
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + strings.TrimSpace(str("body")) + "\n"
	_ = os.MkdirAll(dir, 0o755)
	err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
	sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
}

func skillsRemove(w http.ResponseWriter, root string, str func(string) string) {
	for _, s := range agent.LoadSkills(root) {
		if s.Name == str("name") && (strings.HasPrefix(s.Dir, filepath.Join(root, core.InfuseDirName)) || strings.HasPrefix(s.Dir, core.InfuseHome())) {
			_ = os.RemoveAll(s.Dir)
		}
	}
	sendJSON(w, 200, map[string]any{"ok": true})
}
