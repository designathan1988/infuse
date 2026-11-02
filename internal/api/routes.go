package api

// The API's access table. Every /api/ route is listed here with the methods it takes and how it is
// authorized; a path that is not listed does not exist, and a route is token-only unless the table says
// otherwise. State-changing routes never list GET, so a link, an <img> or a local process cannot reach
// them; only streams and images, which the browser cannot give a header, take the token in the query.

import (
	"crypto/subtle"
	"infuse/internal/config"
	"infuse/internal/core"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

type routeAccess int

const (
	accessToken  routeAccess = iota // the X-Infuse header
	accessStream                    // the header, or ?token= where the browser cannot send one
	accessPublic                    // no token: read-only and free of secrets
)

// apiRoute is one row of the table. A "*" in the pattern stands for exactly one path segment.
// needsRoot marks routes that work inside the project: without a root they would resolve paths
// against the server's own folder.
type apiRoute struct {
	pattern   string
	methods   string
	access    routeAccess
	needsRoot bool
}

var apiRoutes = []apiRoute{
	{pattern: "/api/i18n", methods: "GET", access: accessPublic},

	{pattern: "/api/status", methods: "GET"}, // not public: it carries the agent CLI's environment
	{pattern: "/api/projects", methods: "GET PUT"},
	{pattern: "/api/projects/add", methods: "POST"},
	{pattern: "/api/projects/remove", methods: "POST"},
	{pattern: "/api/projectmode", methods: "GET"},
	{pattern: "/api/fs", methods: "GET"},
	{pattern: "/api/dialog/folder", methods: "POST"},
	{pattern: "/api/dialog/files", methods: "POST"},
	{pattern: "/api/reveal", methods: "POST"},
	{pattern: "/api/open", methods: "POST"},
	{pattern: "/api/install", methods: "POST"},
	{pattern: "/api/uninstall", methods: "POST"},

	{pattern: "/api/settings", methods: "GET PUT"},
	{pattern: "/api/settings/full", methods: "GET"},
	{pattern: "/api/settings/layers", methods: "GET", needsRoot: true},
	{pattern: "/api/settings/local", methods: "PUT", needsRoot: true},
	{pattern: "/api/output-styles", methods: "GET", needsRoot: true},
	{pattern: "/api/turnlog", methods: "GET", needsRoot: true},
	{pattern: "/api/agents/kinds", methods: "GET", needsRoot: true},
	{pattern: "/api/memory/note", methods: "POST", needsRoot: true},
	{pattern: "/api/plugins", methods: "GET", needsRoot: true},
	{pattern: "/api/plugins/install", methods: "POST", needsRoot: true},
	{pattern: "/api/plugins/approve", methods: "POST", needsRoot: true},
	{pattern: "/api/plugins/remove", methods: "POST", needsRoot: true},
	{pattern: "/api/plugins/marketplace", methods: "GET", needsRoot: true},
	{pattern: "/api/session/*/statusline", methods: "GET", needsRoot: true},
	{pattern: "/api/settings/test", methods: "POST"},
	{pattern: "/api/connections", methods: "GET"},
	{pattern: "/api/connections/connect", methods: "POST"},
	{pattern: "/api/connections/disconnect", methods: "POST"},
	{pattern: "/api/connections/default", methods: "POST"},
	{pattern: "/api/providers/models", methods: "POST"},
	{pattern: "/api/models", methods: "GET"},
	{pattern: "/api/usage", methods: "GET"},
	{pattern: "/api/editors", methods: "GET"},
	{pattern: "/api/apps", methods: "GET"},
	{pattern: "/api/apps/open", methods: "POST"},
	{pattern: "/api/apps/command", methods: "POST"},

	{pattern: "/api/rules", methods: "GET PUT"},
	{pattern: "/api/rules/test", methods: "POST"},
	{pattern: "/api/rules/compiled", methods: "GET"},
	{pattern: "/api/base", methods: "GET PUT"},
	{pattern: "/api/base/scaffold", methods: "POST"},
	{pattern: "/api/policy", methods: "GET PUT"},
	{pattern: "/api/bans", methods: "GET"},
	{pattern: "/api/docs", methods: "GET"},
	{pattern: "/api/docs/pin", methods: "POST"},
	{pattern: "/api/docs/unpin", methods: "POST"},
	{pattern: "/api/conventions", methods: "GET POST"},
	{pattern: "/api/hooks", methods: "GET"},
	{pattern: "/api/hooks/add", methods: "POST"},
	{pattern: "/api/hooks/approve", methods: "POST"},
	{pattern: "/api/hooks/remove", methods: "POST"},
	{pattern: "/api/skills", methods: "GET"},
	{pattern: "/api/skills/save", methods: "POST"},
	{pattern: "/api/skills/remove", methods: "POST"},
	{pattern: "/api/commands", methods: "GET"},
	{pattern: "/api/commands/save", methods: "POST"},
	{pattern: "/api/commands/delete", methods: "POST"},
	{pattern: "/api/mcp", methods: "GET"},
	{pattern: "/api/mcp/*", methods: "POST"},

	{pattern: "/api/journal", methods: "GET", needsRoot: true},
	{pattern: "/api/lessons", methods: "GET", needsRoot: true},
	{pattern: "/api/map", methods: "GET POST", needsRoot: true},
	{pattern: "/api/watcher", methods: "GET", needsRoot: true},
	{pattern: "/api/notes", methods: "GET PUT", needsRoot: true},
	{pattern: "/api/notes/list", methods: "GET", needsRoot: true},
	{pattern: "/api/memory", methods: "GET PUT", needsRoot: true},
	{pattern: "/api/memory/items", methods: "GET", needsRoot: true},
	{pattern: "/api/memory/items/save", methods: "POST", needsRoot: true},
	{pattern: "/api/oracle", methods: "GET PUT", needsRoot: true},
	{pattern: "/api/oracle/spec", methods: "GET PUT", needsRoot: true},
	{pattern: "/api/oracle/run", methods: "POST", needsRoot: true},
	{pattern: "/api/oracle/suggest", methods: "GET", needsRoot: true},
	{pattern: "/api/oracle/verdicts", methods: "GET", needsRoot: true},
	{pattern: "/api/oracle/evidence", methods: "GET", needsRoot: true},
	{pattern: "/api/shot", methods: "GET", access: accessStream, needsRoot: true},

	{pattern: "/api/file", methods: "GET PUT", needsRoot: true},
	{pattern: "/api/files/list", methods: "GET", needsRoot: true},
	{pattern: "/api/files/tree", methods: "GET", needsRoot: true},
	{pattern: "/api/frameable", methods: "GET"},
	{pattern: "/api/websearch", methods: "GET"},
	{pattern: "/api/changes", methods: "GET", needsRoot: true},
	{pattern: "/api/diff", methods: "GET", needsRoot: true},
	{pattern: "/api/git", methods: "GET POST", needsRoot: true},
	{pattern: "/api/git/hunk", methods: "POST", needsRoot: true},
	{pattern: "/api/git/branch", methods: "POST", needsRoot: true},
	{pattern: "/api/pr", methods: "GET POST", needsRoot: true},
	{pattern: "/api/devservers", methods: "GET POST"},

	{pattern: "/api/tasks", methods: "GET PUT"},
	{pattern: "/api/tasks/run", methods: "POST"},
	{pattern: "/api/tasks/delete", methods: "POST"},

	{pattern: "/api/sessions", methods: "GET POST", needsRoot: true},
	{pattern: "/api/sessions/new", methods: "POST", needsRoot: true},
	{pattern: "/api/sessions/all", methods: "GET"},
	{pattern: "/api/sessions/search", methods: "GET"},
	{pattern: "/api/sessions/*", methods: "GET PUT DELETE", needsRoot: true},
	{pattern: "/api/sessions/*/send", methods: "POST", needsRoot: true},
	{pattern: "/api/sessions/*/stop", methods: "POST", needsRoot: true},
	{pattern: "/api/sessions/*/permission", methods: "POST", needsRoot: true},
	{pattern: "/api/sessions/*/answer", methods: "POST", needsRoot: true},
	{pattern: "/api/sessions/*/events", methods: "GET", access: accessStream, needsRoot: true},
	{pattern: "/api/session/*/settings", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/pin", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/archive", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/unarchive", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/rewind", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/summarize", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/fork", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/pinmsg", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/btw", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/review", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/suggest", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/comments", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/mission", methods: "POST", needsRoot: true},
	{pattern: "/api/session/*/context", methods: "GET", needsRoot: true},
	{pattern: "/api/session/*/bans", methods: "GET POST", needsRoot: true},
	{pattern: "/api/session/*/procs", methods: "GET POST", needsRoot: true},

	{pattern: "/api/term/open", methods: "POST"},
	{pattern: "/api/term/*/stream", methods: "GET", access: accessStream},
	{pattern: "/api/term/*/input", methods: "POST"},
	{pattern: "/api/term/*/resize", methods: "POST"},
	{pattern: "/api/term/*/close", methods: "POST"},
	{pattern: "/api/view/stream", methods: "GET", access: accessStream},
	{pattern: "/api/view/open", methods: "POST"},
	{pattern: "/api/view/agent", methods: "POST"},
	{pattern: "/api/view/input", methods: "POST"},
}

// findRoute returns the table row of a path, preferring an exact row over a wildcard one.
func findRoute(path string) *apiRoute {
	var wild *apiRoute
	for i := range apiRoutes {
		rt := &apiRoutes[i]
		if rt.pattern == path {
			return rt
		}
		if wild == nil && strings.Contains(rt.pattern, "*") && patternMatches(rt.pattern, path) {
			wild = rt
		}
	}
	return wild
}

func patternMatches(pattern, path string) bool {
	want, got := strings.Split(pattern, "/"), strings.Split(path, "/")
	if len(want) != len(got) {
		return false
	}
	for i := range want {
		if want[i] != got[i] && (want[i] != "*" || got[i] == "") {
			return false
		}
	}
	return true
}

func (rt *apiRoute) allows(method string) bool {
	for _, m := range strings.Fields(rt.methods) {
		if m == method {
			return true
		}
	}
	return false
}

func tokenEqual(got, token string) bool {
	return token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func (rt *apiRoute) authorized(r *http.Request, token string) bool {
	switch rt.access {
	case accessPublic:
		return true
	case accessStream:
		return tokenEqual(r.Header.Get("X-Infuse"), token) || tokenEqual(r.URL.Query().Get("token"), token)
	}
	return tokenEqual(r.Header.Get("X-Infuse"), token)
}

// guardAPI puts the table in front of the handlers: origin, token, method, project, in that order. A
// caller without the token gets the same 403 for a real route and an invented one.
func guardAPI(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := sameOrigin(r.Host, r.Header.Get("Origin"), r.Method, r.Header.Get("X-Infuse"), token); err != nil {
			sendJSON(w, http.StatusForbidden, map[string]any{"error": err.Error()})
			return
		}
		rt := findRoute(r.URL.Path)
		if (rt == nil && !tokenEqual(r.Header.Get("X-Infuse"), token)) || (rt != nil && !rt.authorized(r, token)) {
			sendJSON(w, http.StatusForbidden, map[string]any{"error": "request without the application token"})
			return
		}
		if rt == nil {
			sendJSON(w, http.StatusNotFound, map[string]any{"error": "unknown route", "code": "api.unknownRoute"})
			return
		}
		if !rt.allows(r.Method) {
			w.Header().Set("Allow", strings.ReplaceAll(rt.methods, " ", ", "))
			sendJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed", "code": "api.methodNotAllowed"})
			return
		}
		if code, res := checkRoot(rt, r.URL.Query()); res != nil {
			sendJSON(w, code, res)
			return
		}
		next(w, r)
	}
}

// checkRoot refuses a folder the user never opened: root and project reach the file routes, so
// they must name a registered project or a folder inside one.
func checkRoot(rt *apiRoute, q url.Values) (int, map[string]any) {
	if rt.needsRoot && strings.TrimSpace(q.Get("root")) == "" {
		return http.StatusBadRequest, map[string]any{"error": "the project folder is missing", "code": "project.chooseRoot"}
	}
	for _, k := range []string{"root", "project"} {
		if p := q.Get(k); p != "" && !registeredRoot(p) {
			return http.StatusForbidden, map[string]any{"error": "the folder is not a registered project", "code": "project.notRegistered"}
		}
	}
	return 0, nil
}

// registeredRoot says whether p is a registered project, a folder inside one (a session's worktree
// by default) or inside the folder the project keeps its worktrees in, which settings can move out.
func registeredRoot(p string) bool {
	key := config.RootKey(p)
	projects := config.LoadProjects()
	for _, e := range projects {
		if config.RootKey(e.Root) == key || core.Within(e.Root, p) {
			return true
		}
	}
	for _, e := range projects {
		if core.Within(filepath.Join(e.Root, filepath.FromSlash(config.EffectiveSettings(e.Root).WorktreeDir)), p) {
			return true
		}
	}
	return false
}
