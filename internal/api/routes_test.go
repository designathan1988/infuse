package api

import (
	"infuse/internal/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const testToken = "s3cr3t-token"

var everyMethod = []string{"GET", "HEAD", "OPTIONS", "POST", "PUT", "PATCH", "DELETE"}

// call runs a request through the guard; reached says whether the handler was let through.
func call(t *testing.T, method, target, marker string) (code int, reached bool) {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	r.Host = "127.0.0.1:7470"
	if marker != "" {
		r.Header.Set("X-Infuse", marker)
	}
	w := httptest.NewRecorder()
	guardAPI(testToken, func(http.ResponseWriter, *http.Request) { reached = true })(w, r)
	return w.Code, reached
}

// projectQuery registers a throwaway project and returns the query naming it.
func projectQuery(t *testing.T) (string, string) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	config.RememberProject(root)
	return root, "?root=" + url.QueryEscape(root)
}

func concrete(pattern string) string { return strings.ReplaceAll(pattern, "*", "x1") }

// The table is walked in full: nothing but the public rows answers without the token, and no row
// answers a method it does not list.
func TestEveryRouteRequiresTheTokenAndItsMethod(t *testing.T) {
	_, query := projectQuery(t)
	for _, rt := range apiRoutes {
		target := concrete(rt.pattern) + query
		for _, m := range everyMethod {
			code, reached := call(t, m, target, "")
			if rt.access == accessPublic {
				if rt.allows(m) != reached {
					t.Errorf("%s %s without token: reached=%v", m, rt.pattern, reached)
				}
				continue
			}
			if reached || code != http.StatusForbidden {
				t.Errorf("%s %s without token: %d reached=%v", m, rt.pattern, code, reached)
			}
			code, reached = call(t, m, target, testToken)
			if rt.allows(m) != reached {
				t.Errorf("%s %s with token: reached=%v", m, rt.pattern, reached)
			}
			if !rt.allows(m) && code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s with token: %d, want 405", m, rt.pattern, code)
			}
		}
	}
}

func TestOnlyTheCatalogIsPublic(t *testing.T) {
	for _, rt := range apiRoutes {
		if rt.access == accessPublic && rt.pattern != "/api/i18n" {
			t.Errorf("%s is public; a new public route must be justified here", rt.pattern)
		}
		if rt.access == accessPublic && rt.methods != "GET" {
			t.Errorf("%s is public and writable", rt.pattern)
		}
	}
}

// Each of these changed state on a token-free GET.
func TestStateChangingRoutesRefuseGET(t *testing.T) {
	_, query := projectQuery(t)
	for _, p := range []string{
		"/api/install", "/api/uninstall", "/api/sessions/x1/stop", "/api/session/x1/archive", "/api/session/x1/pin",
		"/api/session/x1/unarchive", "/api/sessions/new", "/api/term/open", "/api/term/x1/close", "/api/mcp/restart",
		"/api/dialog/folder", "/api/session/x1/review", "/api/session/x1/suggest", "/api/session/x1/mission",
		"/api/settings/test", "/api/tasks/run", "/api/tasks/delete", "/api/oracle/run", "/api/git/hunk", "/api/git/branch",
	} {
		rt := findRoute(p)
		if rt == nil {
			t.Fatalf("%s is not in the table", p)
		}
		if rt.allows("GET") {
			t.Errorf("%s lists GET", p)
		}
		if code, reached := call(t, "GET", p+query, ""); reached || code != http.StatusForbidden {
			t.Errorf("GET %s without token: %d reached=%v", p, code, reached)
		}
		if code, reached := call(t, "GET", p+query, testToken); reached || code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s with token: %d reached=%v", p, code, reached)
		}
	}
}

// Only what the browser cannot give a header takes the token in the query.
func TestQueryTokenOnlyOnStreams(t *testing.T) {
	_, query := projectQuery(t)
	streams := map[string]bool{"/api/sessions/*/events": true, "/api/term/*/stream": true, "/api/view/stream": true, "/api/shot": true}
	for _, rt := range apiRoutes {
		if rt.access == accessPublic {
			continue
		}
		method := strings.Fields(rt.methods)[0]
		target := concrete(rt.pattern) + query + "&token=" + testToken
		code, reached := call(t, method, target, "")
		if streams[rt.pattern] != reached {
			t.Errorf("%s %s with a query token: %d reached=%v", method, rt.pattern, code, reached)
		}
		if _, reached = call(t, method, concrete(rt.pattern)+query+"&token=wrong", ""); reached {
			t.Errorf("%s %s accepted a wrong query token", method, rt.pattern)
		}
	}
}

func TestUnknownAndRemovedRoutesDoNotExist(t *testing.T) {
	_, query := projectQuery(t)
	for _, p := range []string{"/api/nope", "/api/files/read", "/api/terminal", "/api/sessions/x1/revert", "/api/sessions/x1/duplicate", "/api/session/x1", "/api/mcp/a/b"} {
		for _, m := range []string{"GET", "POST"} {
			if code, reached := call(t, m, p+query, testToken); reached || code != http.StatusNotFound {
				t.Errorf("%s %s: %d reached=%v", m, p, code, reached)
			}
			if code, reached := call(t, m, p+query, ""); reached || code != http.StatusForbidden {
				t.Errorf("%s %s without token: %d reached=%v, must not tell an unknown route apart", m, p, code, reached)
			}
		}
	}
}

func TestRootMustBeARegisteredProject(t *testing.T) {
	root, query := projectQuery(t)
	stranger := t.TempDir()
	inside := filepath.Join(root, ".infuse", "worktrees", "w1")
	for _, tc := range []struct {
		target string
		code   int
		ok     bool
	}{
		{"/api/journal" + query, 200, true},
		{"/api/diff?root=" + url.QueryEscape(inside), 200, true},
		{"/api/journal?root=" + url.QueryEscape(stranger), 403, false},
		{"/api/file?path=a&root=" + url.QueryEscape(stranger), 403, false},
		{"/api/journal?root=" + url.QueryEscape(filepath.Join(root, "..")), 403, false},
		{"/api/diff?root=" + url.QueryEscape(root) + "&project=" + url.QueryEscape(stranger), 403, false},
		{"/api/journal", 400, false},
		{"/api/file?path=go.mod", 400, false},
		{"/api/settings", 200, true}, // a global route needs no project
	} {
		if code, reached := call(t, "GET", tc.target, testToken); code != tc.code || reached != tc.ok {
			t.Errorf("%s: %d reached=%v, want %d %v", tc.target, code, reached, tc.code, tc.ok)
		}
	}
}

func TestWorktreeFolderOutsideTheProjectIsAccepted(t *testing.T) {
	root, _ := projectQuery(t)
	s := config.LoadSettings()
	s.WorktreeDir = "../outside-worktrees"
	if err := config.SaveSettings(s); err != nil {
		t.Fatal(err)
	}
	if !registeredRoot(filepath.Join(root, "..", "outside-worktrees", "w1")) {
		t.Fatal("a session worktree in the configured folder was refused")
	}
	if registeredRoot(filepath.Join(root, "..", "elsewhere")) {
		t.Fatal("a sibling folder was accepted")
	}
}

// goRoutes are the path literals the handlers dispatch on.
func goRoutes(t *testing.T) []string {
	t.Helper()
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`"(/api/[a-z0-9/]+)"`)
	var out []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || f == "routes.go" {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			out = append(out, m[1])
		}
	}
	return out
}

func familyHasRow(prefix string) bool {
	for _, rt := range apiRoutes {
		if strings.HasPrefix(rt.pattern, prefix) {
			return true
		}
	}
	return false
}

// A handler added without a table row would answer 404 forever; this names it instead.
func TestEveryHandlerPathIsInTheTable(t *testing.T) {
	seen := goRoutes(t)
	if len(seen) < 40 {
		t.Fatalf("the source scan found only %d routes", len(seen))
	}
	for _, p := range seen {
		if strings.HasSuffix(p, "/") { // a family dispatched by prefix: its rows are wildcards
			if !familyHasRow(p) {
				t.Errorf("no row serves the family %s", p)
			}
			continue
		}
		if findRoute(p) == nil {
			t.Errorf("handler path %s has no table row", p)
		}
	}
}

// uiRoutes are the API paths the interface calls, with template parts as one wildcard segment.
func uiRoutes(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	re := regexp.MustCompile("[`'\"](/api/[^'\"`?]*)")
	tpl := regexp.MustCompile(`\$\{[^}]*\}`)
	err := filepath.WalkDir(filepath.Join("..", "..", "ui", "src"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".js") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			route := tpl.ReplaceAllString(m[1], "x1")
			if strings.HasSuffix(route, "/") { // '/api/mcp/' + name
				route += "x1"
			}
			out[route] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestEveryInterfaceCallHasARoute(t *testing.T) {
	calls := uiRoutes(t)
	if len(calls) < 40 {
		t.Fatalf("the source scan found only %d calls", len(calls))
	}
	for p := range calls {
		if findRoute(p) == nil {
			t.Errorf("the interface calls %s, which is not in the table", p)
		}
	}
}

// Routes no interface code calls are kept on purpose; anything new with no caller fails here.
var keptWithoutCaller = map[string]string{
	"/api/status":           "scratch and audit scripts",
	"/api/projects/add":     "scratch and audit scripts register the fixture project",
	"/api/fs":               "in-app folder browser backend",
	"/api/reveal":           "external callers",
	"/api/settings/test":    "external callers",
	"/api/providers/models": "external callers",
	"/api/rules/compiled":   "external callers",
	"/api/frameable":        "external callers",
	"/api/websearch":        "external callers",
	"/api/watcher":          "external callers",
	"/api/notes":            "external callers",
	"/api/notes/list":       "external callers",
	"/api/agents/kinds":     "extensions API, appExtensions.go",
	"/api/sessions":         "compatibility family in docs/ARCHITECTURE.md",
}

func TestDeadRoutesAreDeclared(t *testing.T) {
	calls := uiRoutes(t)
	called := map[string]bool{}
	for p := range calls {
		if rt := findRoute(p); rt != nil {
			called[rt.pattern] = true
		}
	}
	for _, rt := range apiRoutes {
		if !called[rt.pattern] && keptWithoutCaller[rt.pattern] == "" {
			t.Errorf("%s has no caller and no reason to stay", rt.pattern)
		}
		if called[rt.pattern] && keptWithoutCaller[rt.pattern] != "" {
			t.Errorf("%s has a caller now: take it off keptWithoutCaller", rt.pattern)
		}
	}
}

func TestSettingsNeverCarryTheKey(t *testing.T) {
	for _, key := range []string{"sk-ant-api03-abcdefghijklmnop", "short"} {
		got := maskSettings(config.Settings{APIKey: key}).APIKey
		if got != keyMask {
			t.Errorf("key %q shown as %q", key, got)
		}
	}
	if got := maskSettings(config.Settings{}).APIKey; got != "" {
		t.Errorf("no key shown as %q", got)
	}
}
