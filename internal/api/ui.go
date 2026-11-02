package api

// The application: HTTP on 127.0.0.1 serving the embedded ui/index.html and a JSON API, shown inside the
// native window (window_*.go). Nothing leaves the machine. `infuse serve` serves without a window.

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/browser"
	"infuse/internal/desktop"
	"infuse/internal/mcp"
	"infuse/internal/platform/process"
	uiembed "infuse/ui"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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

// uiMux routes the embedded UI and the API; every API call must come from the app itself.
func uiMux(token string) *http.ServeMux {
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
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if err := sameOrigin(r.Host, r.Header.Get("Origin"), r.Method, r.Header.Get("X-Infuse"), token); err != nil {
			sendJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		// Reading an arbitrary file or fetching an arbitrary URL is as sensitive as a mutation: the
		// application token is required even on GET, so a local process (the agent's shell included)
		// cannot use the API as its own file or network oracle.
		if sensitiveRoute(r.URL.Path) && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Infuse")), []byte(token)) != 1 {
			sendJSON(w, http.StatusForbidden, map[string]any{"error": "request without the application token"})
			return
		}
		apiHandler(w, r)
	})
	return mux
}

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
	// The token is the app's own key to its API. Other sites cannot send it (CSRF), rebound hostnames are
	// refused (DNS rebinding), and local processes — the agent's own Bash included — do not know it, so
	// they cannot approve permissions, edit the oracle or run the executor through the API.
	token := apiToken()
	mux := uiMux(token)
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
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	// the same cleanup the native window does: no orphan browser, process or connector on exit
	browser.CloseAgentBrowsers()
	closeViewBrowsers()
	agent.StopAllProcs()
	mcp.CloseAll()
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

	if appCoreRoutes(w, r, body, root, str, qm) || workspaceRoutes(w, r, body, root, str, qm) {
		return
	}
	http.NotFound(w, r)
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
