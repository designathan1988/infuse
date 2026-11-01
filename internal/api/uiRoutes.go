package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/projectmap"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// appCoreRoutes are the app's own routes that are not grouped by resource: status, project registry,
// dialogs, rules and filter installation.
func appCoreRoutes(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string, qmap map[string]string) bool {
	switch r.URL.Path {
	case "/api/status":
		sendJSON(w, 200, filter.GetStatus())
	case "/api/projects", "/api/projects/remove", "/api/projects/add":
		projectsRoute(w, r, body, str)
	case "/api/fs":
		sendJSON(w, 200, listFolders(r.URL.Query().Get("path")))
	case "/api/dialog/folder", "/api/dialog/files":
		dialogRoute(w, r, str)
	case "/api/reveal":
		_ = desktop.RevealInFileManager(str("path"))
		sendJSON(w, 200, map[string]any{"ok": true})
	case "/api/install":
		sendJSON(w, 200, filter.Install())
	case "/api/uninstall":
		sendJSON(w, 200, filter.Uninstall())
	case "/api/rules", "/api/rules/test":
		rulesRoute(w, r, body, root, str, qmap)
	case "/api/policy":
		policyRoute(w, r, body, root, qmap)
	case "/api/base", "/api/base/scaffold":
		baseRoute(w, r, body, root, str)
	default:
		return false
	}
	return true
}

func projectsRoute(w http.ResponseWriter, r *http.Request, body map[string]any, str func(string) string) {
	switch r.URL.Path {
	case "/api/projects":
		if r.Method == http.MethodPut { // reorder / rename: the whole list, as the sidebar shows it
			var list []config.ProjectEntry
			b, _ := json.Marshal(body["projects"])
			_ = json.Unmarshal(b, &list)
			if list == nil {
				list = []config.ProjectEntry{}
			}
			_ = config.SaveProjects(list)
		}
		sendJSON(w, 200, config.LoadProjects())
	case "/api/projects/remove":
		var out []config.ProjectEntry
		key := config.RootKey(str("root"))
		for _, p := range config.LoadProjects() {
			if config.RootKey(p.Root) != key {
				out = append(out, p)
			}
		}
		if out == nil {
			out = []config.ProjectEntry{}
		}
		_ = config.SaveProjects(out)
		sendJSON(w, 200, out)
	case "/api/projects/add":
		raw := strings.TrimSpace(str("root"))
		p := filepath.Clean(raw)
		if raw == "" || p == "." || p == string(filepath.Separator) {
			sendJSON(w, 400, map[string]any{"error": "the project folder is missing", "code": "project.noRoot"})
			return
		}
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "the project folder was not found", "code": "project.notFound"})
			return
		}
		config.RememberProject(p)
		sendJSON(w, 200, map[string]any{"ok": true, "root": p})
	}
}

func dialogRoute(w http.ResponseWriter, r *http.Request, str func(string) string) {
	if r.URL.Path == "/api/dialog/folder" {
		// Native dialog from the server side: used when the page is not inside the native window.
		p, err := desktop.OSDialogs(nil).PickFolder(core.M("dialog.chooseFolder").Render(config.LoadSettings().Language))
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		if p != "" {
			config.RememberProject(p)
		}
		sendJSON(w, 200, map[string]any{"path": p})
		return
	}
	list, err := desktop.OSDialogs(nil).PickFiles(core.M("dialog.attachFiles").Render(config.LoadSettings().Language))
	if err != nil {
		sendJSON(w, 500, map[string]any{"error": err.Error()})
		return
	}
	out := []core.Attachment{}
	for _, p := range list {
		if a, ok := desktop.AttachmentOf(p); ok {
			out = append(out, a)
		}
	}
	sendJSON(w, 200, out)
}

func rulesRoute(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string, q map[string]string) {
	if r.URL.Path == "/api/rules/test" {
		rulesTest(w, root, str)
		return
	}
	file := filter.ProjectRulesFile(root)
	if q["scope"] == "global" {
		file = filter.GlobalRulesFile()
	}
	if r.Method == http.MethodPut {
		var rf filter.RulesFile
		b, _ := json.Marshal(body)
		_ = json.Unmarshal(b, &rf)
		if rf.Rules == nil {
			rf.Rules = []filter.Rule{}
		}
		if rf.Disable == nil {
			rf.Disable = []string{}
		}
		if err := filter.SaveRulesFile(file, rf); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(w, 200, map[string]any{"ok": true, "file": file})
		return
	}
	rf, _ := filter.LoadRulesFile(file)
	sendJSON(w, 200, map[string]any{"file": file, "exists": core.Exists(file), "rules": rf.Rules, "disable": rf.Disable})
}

func rulesTest(w http.ResponseWriter, root string, str func(string) string) {
	// Dry run: what the filter would do with this command under the current rules. Nothing is executed.
	set := filter.LoadRules(root)
	command := str("command")
	var notes []string
	if strings.Contains(command, "<<") && !set.Disabled["heredoc"] {
		scratch := filepath.Join(core.InfuseHome(), "preview")
		_ = os.MkdirAll(scratch, 0o755)
		tr := filter.TransformCommand(command, scratch)
		command = tr.Command
		for _, a := range tr.Actions {
			if a.Kind == filter.ActLesson {
				notes = append(notes, fmt.Sprintf("[infuse] vice %s: %s", a.Vice, a.Detail))
			} else {
				notes = append(notes, "[infuse] "+a.Detail)
			}
		}
	}
	final, lines := filter.ApplyCommandRules(root, set, command)
	sendJSON(w, 200, map[string]any{"finalCommand": final, "lines": append(notes, lines...)})
}

// workspaceRoutes expose the project's journal, map, watcher and notes.
func workspaceRoutes(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string, q map[string]string) bool {
	switch r.URL.Path {
	case "/api/journal":
		sendJSON(w, 200, reverse(core.ReadJSONL(filepath.Join(root, core.InfuseDirName, "journal.jsonl")), 300))
	case "/api/lessons":
		sendJSON(w, 200, reverse(core.ReadJSONL(filepath.Join(root, core.InfuseDirName, "lessons.jsonl")), 300))
	case "/api/map":
		var m *projectmap.ProjectMap
		if q["refresh"] != "" {
			m = projectmap.BuildMap(root)
			projectmap.WriteMap(root, m)
		} else {
			m = projectmap.LoadOrBuildMap(root)
		}
		sendJSON(w, 200, m)
	case "/api/watcher":
		pid, alive := filter.WatcherAlive(root)
		sendJSON(w, 200, map[string]any{"pid": pid, "alive": alive})
	case "/api/notes", "/api/notes/list":
		notesRoute(w, r, root, str)
	default:
		return false
	}
	return true
}

func notesRoute(w http.ResponseWriter, r *http.Request, root string, str func(string) string) {
	dir := filepath.Join(root, core.InfuseDirName, "notes")
	if r.URL.Path == "/api/notes/list" {
		entries, _ := os.ReadDir(dir)
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sendJSON(w, 200, names)
		return
	}
	name := reSafeName.ReplaceAllString(r.URL.Query().Get("name"), "_")
	file := filepath.Join(dir, name)
	if r.Method == http.MethodPut {
		_ = os.MkdirAll(filepath.Dir(file), 0o755)
		_ = os.WriteFile(file, []byte(str("content")), 0o644)
		sendJSON(w, 200, map[string]any{"ok": true, "file": file})
		return
	}
	b, _ := os.ReadFile(file)
	sendJSON(w, 200, map[string]any{"file": file, "content": string(b)})
}
