package api

// The application: HTTP on 127.0.0.1 serving the embedded ui/index.html and a JSON API, shown inside the
// native window (window_*.go). Nothing leaves the machine. `infuse serve` serves without a window.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/browser"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/mcp"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	uiembed "infuse/ui"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// uiFile serves one embedded file; INFUSE_UI_DIR serves ui/ from disk instead (development).
func uiFile(w http.ResponseWriter, name string) {
	var b []byte
	var err error
	if dev := os.Getenv("INFUSE_UI_DIR"); dev != "" {
		b, err = os.ReadFile(filepath.Join(dev, filepath.FromSlash(name)))
	} else {
		b, err = uiembed.FS.ReadFile(name)
	}
	if err != nil {
		http.Error(w, "not found", 404)
		return
	}
	switch {
	case strings.HasSuffix(name, ".html"):
		w.Header().Set("content-type", "text/html; charset=utf-8")
	case strings.HasSuffix(name, ".js"):
		w.Header().Set("content-type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(name, ".css"):
		w.Header().Set("content-type", "text/css; charset=utf-8")
	case strings.HasSuffix(name, ".json"):
		w.Header().Set("content-type", "application/json; charset=utf-8")
	}
	w.Header().Set("cache-control", "no-cache")
	_, _ = w.Write(b)
}

// Start runs the application server and, when window is true, its native window.
func Start(window bool) { StartUI(window) }

func StartUI(window bool) {
	addr := "127.0.0.1:0"
	if p := os.Getenv("INFUSE_PORT"); p != "" {
		addr = "127.0.0.1:" + p
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "infuse: cannot listen:", err)
		os.Exit(1)
	}
	url := "http://" + ln.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if strings.Contains(name, "..") {
			http.NotFound(w, r)
			return
		}
		uiFile(w, name)
	})
	mux.HandleFunc("/icon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "image/png")
		_, _ = w.Write(desktop.IconPNG(64))
	})
	// The token is the app's own key to its API. Other sites cannot send it (CSRF), rebound hostnames are
	// refused (DNS rebinding), and local processes — the agent's own Bash included — do not know it, so
	// they cannot approve permissions, edit the oracle or run the executor through the API.
	token := apiToken()
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if err := sameOrigin(r.Host, r.Header.Get("Origin"), r.Method, r.Header.Get("X-Infuse"), token); err != nil {
			sendJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		apiHandler(w, r)
	})
	go func() { _ = http.Serve(ln, mux) }()
	StartScheduler()
	go agent.ResumeInterruptedSessions() // turns that infuse stopped by closing or restarting go on
	// the token travels in the URL fragment, which never leaves the window (not sent in requests)
	if window {
		desktop.RunWindow(url + "/#token=" + token)
		browser.CloseAgentBrowsers()
		closeViewBrowsers()
		agent.StopAllProcs()
		mcp.CloseAll()
		os.Exit(0)
	}
	process.AttachParentConsole()
	fmt.Println("[infuse] " + url + "/#token=" + token)
	select {}
}

func sendJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

var reSafeName = regexp.MustCompile(`[^\w.-]`)

func apiHandler(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	root := q.Get("root")
	var body map[string]any
	if r.Method == http.MethodPost || r.Method == http.MethodPut {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	str := func(k string) string { s, _ := body[k].(string); return s }
	qm := map[string]string{}
	for k := range q {
		qm[k] = q.Get(k)
	}
	if appAPI(w, r, qm, body) || studioAPI(w, r, qm, body) {
		return
	}

	switch r.URL.Path {
	case "/api/status":
		sendJSON(w, 200, filter.GetStatus())
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

		for _, p := range config.LoadProjects() {
			if p.Root != str("root") {
				out = append(out, p)
			}
		}
		if out == nil {
			out = []config.ProjectEntry{}
		}
		_ = config.SaveProjects(out)
		sendJSON(w, 200, out)
	case "/api/projects/add":
		p := filepath.Clean(str("root"))
		if st, err := os.Stat(p); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "pasta não encontrada"})
			return
		}
		config.RememberProject(p)
		sendJSON(w, 200, map[string]any{"ok": true, "root": p})
	case "/api/fs":
		// Folder listing for the in-app folder browser (no native dialog): "" lists the roots (drives / home).
		sendJSON(w, 200, listFolders(q.Get("path")))
	case "/api/dialog/folder":
		// Native dialog from the server side: used when the page is not inside the native window.
		p, err := desktop.OSDialogs(nil).PickFolder("Abrir pasta do projeto")
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return
		}
		if p != "" {
			config.RememberProject(p)
		}
		sendJSON(w, 200, map[string]any{"path": p})
	case "/api/dialog/files":
		list, err := desktop.OSDialogs(nil).PickFiles("Anexar arquivos")
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
	case "/api/reveal":
		_ = desktop.RevealInFileManager(str("path"))
		sendJSON(w, 200, map[string]any{"ok": true})
	case "/api/install":
		sendJSON(w, 200, filter.Install())
	case "/api/uninstall":
		sendJSON(w, 200, filter.Uninstall())
	case "/api/rules":
		file := filter.ProjectRulesFile(root)
		if q.Get("scope") == "global" {
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
	case "/api/rules/test":
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
				if a.Kind ==
					filter.ActLesson {
					notes = append(notes, fmt.Sprintf("[infuse] vice %s: %s", a.Vice, a.Detail))
				} else {
					notes = append(notes, "[infuse] "+a.Detail)
				}
			}
		}
		final, lines := filter.ApplyCommandRules(root, set, command)
		sendJSON(w, 200, map[string]any{"finalCommand": final, "lines": append(notes, lines...)})
	case "/api/journal":
		sendJSON(w, 200, reverse(core.ReadJSONL(filepath.Join(root, core.InfuseDirName, "journal.jsonl")), 300))
	case "/api/lessons":
		sendJSON(w, 200, reverse(core.ReadJSONL(filepath.Join(root, core.InfuseDirName, "lessons.jsonl")), 300))
	case "/api/map":
		var m *projectmap.ProjectMap

		if q.Get("refresh") != "" {
			m = projectmap.BuildMap(root)
			projectmap.WriteMap(root, m)
		} else {
			m = projectmap.LoadOrBuildMap(root)
		}
		sendJSON(w, 200, m)
	case "/api/watcher":
		pid, alive := filter.WatcherAlive(root)
		sendJSON(w, 200, map[string]any{"pid": pid, "alive": alive})
	case "/api/notes":
		name := reSafeName.ReplaceAllString(q.Get("name"), "_")
		file := filepath.Join(root, core.InfuseDirName, "notes", name)
		if r.Method == http.MethodPut {
			_ = os.MkdirAll(filepath.Dir(file), 0o755)
			_ = os.WriteFile(file, []byte(str("content")), 0o644)
			sendJSON(w, 200, map[string]any{"ok": true, "file": file})
			return
		}
		b, _ := os.ReadFile(file)
		sendJSON(w, 200, map[string]any{"file": file, "content": string(b)})
	case "/api/notes/list":
		dir := filepath.Join(root, core.InfuseDirName, "notes")
		entries, _ := os.ReadDir(dir)
		names := []string{}
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		sendJSON(w, 200, names)
	default:
		http.NotFound(w, r)
	}
}

func reverse(list []map[string]any, limit int) []map[string]any {
	if len(list) > limit {
		list = list[len(list)-limit:]
	}
	out := make([]map[string]any, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, list[i])
	}
	return out
}
