package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/mcp"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepCountAndType(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	_ = os.WriteFile(filepath.Join(root, "a.go"), []byte("needle\nneedle\nother\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "b.py"), []byte("needle\n"), 0o644)
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}
	run := func(in string) (string, bool) { return grepTool{}.Run(context.Background(), tc, json.RawMessage(in)) }
	out, isErr := run(`{"pattern":"needle","count":true}`)
	if isErr || !strings.Contains(out, "a.go:2") || !strings.Contains(out, "b.py:1") || !strings.Contains(out, "3 matching") {
		t.Fatal(out)
	}
	out, _ = run(`{"pattern":"needle","type":"go"}`)
	if !strings.Contains(out, "a.go:1:needle") || strings.Contains(out, "b.py") {
		t.Fatal(out)
	}
	out, _ = run(`{"pattern":"needle","type":"py","count":true}`)
	if strings.Contains(out, "a.go") || !strings.Contains(out, "b.py:1") {
		t.Fatal(out)
	}
	if out, isErr := run(`{"pattern":"needle","type":"nosuch"}`); !isErr {
		t.Fatal("an unknown type returned:", out)
	}
	many := strings.Repeat("needle\n", 900)
	_ = os.WriteFile(filepath.Join(root, "big.txt"), []byte(many), 0o644)
	_ = os.WriteFile(filepath.Join(root, "z.txt"), []byte("needle\n"), 0o644)
	if out, _ := run(`{"pattern":"needle","count":true}`); !strings.Contains(out, "z.txt:1") || !strings.Contains(out, "904 matching") {
		t.Fatal("a count stops for no limit:", out)
	}
}

func TestSearchProviderTable(t *testing.T) {
	testHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Key") != "k3y" || r.URL.Query().Get("q") != "go lang" {
			http.Error(w, "no", 403)
			return
		}
		fmt.Fprint(w, `{"web":{"results":[{"title":"T <b>1</b>","url":"https://a.dev","text":"snip"},{"title":"no link"}]}}`)
	}))
	defer srv.Close()
	old := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	defer func() { config.SecretBackend = old }()
	_ = config.SecretSet("search-key", "k3y")
	s := newSession(t.TempDir())
	s.settings.SearchProvider = "mine"
	s.settings.SearchProviders = []config.SearchProvider{{Name: "mine", URL: srv.URL + "/s?q={query}", Results: "web.results", Title: "title", Link: "url", Snippet: "text", Header: "X-Key", KeyVault: "search-key"}}
	res, name, err := s.webSearch(context.Background(), "go lang", 5)
	if err != nil || name != "mine" || len(res) != 1 || res[0].Title != "T 1" || res[0].URL != "https://a.dev" || res[0].Snippet != "snip" {
		t.Fatalf("%v %s %+v", err, name, res)
	}
	s.settings.SearchProvider = "nobody"
	if _, _, err := s.webSearch(context.Background(), "x", 5); err == nil {
		t.Fatal("an unknown provider")
	}
	names := strings.Join(SearchProviderNames(s.settings), " ")
	if !strings.Contains(names, "duckduckgo") || !strings.Contains(names, "bing") || !strings.Contains(names, "mine") {
		t.Fatal(names)
	}
}

// fakeMCP serves the protocol over HTTP: tools, resources and prompts.
func fakeMCP(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m mcp.RPCMessage
		_ = json.NewDecoder(r.Body).Decode(&m)
		if m.ID == nil {
			return
		}
		result := map[string]string{
			"initialize":     `{"protocolVersion":"2025-06-18","capabilities":{}}`,
			"tools/list":     `{"tools":[]}`,
			"resources/list": `{"resources":[{"uri":"file:///a","name":"a","description":"first"}]}`,
			"resources/read": `{"contents":[{"uri":"file:///a","text":"resource body"},{"uri":"file:///b","blob":"aGVsbG8=","mimeType":"image/png"}]}`,
			"prompts/list":   `{"prompts":[{"name":"greet","description":"says hi","arguments":[{"name":"who"}]}]}`,
			"prompts/get":    `{"messages":[{"role":"user","content":{"type":"text","text":"hi there"}}]}`,
		}[m.Method]
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":%s}`, *m.ID, result)
	}))
}

func TestMCPResourceAndPromptTools(t *testing.T) {
	testHome(t)
	root := config.CanonRoot(t.TempDir())
	srv := fakeMCP(t)
	defer srv.Close()
	cfg := mcp.ServerConfig{Type: "http", URL: srv.URL}
	if err := mcp.Edit(root, "project", func(sv map[string]mcp.ServerConfig) { sv["srv"] = cfg }); err != nil {
		t.Fatal(err)
	}
	_ = mcp.Approve(root, "project", "srv", cfg)
	mcp.For(root).Ensure(context.Background())
	defer mcp.For(root).Restart(context.Background(), "project", "gone")
	s := newSession(root)
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}
	run := func(name, in string) (string, bool) {
		tool, ok := s.lookupTool(name)
		if !ok {
			t.Fatal("no tool", name)
		}
		if tool.Category() != "mcp" {
			t.Fatal("connector content goes through the policy:", tool.Category())
		}
		return tool.Run(context.Background(), tc, json.RawMessage(in))
	}
	if out, isErr := run("ListMcpResources", `{}`); isErr || !strings.Contains(out, "file:///a") {
		t.Fatal(out)
	}
	if out, isErr := run("ReadMcpResource", `{"server":"srv","uri":"file:///a"}`); isErr || !strings.Contains(out, "resource body") || !strings.Contains(out, "image/png") {
		t.Fatal(out)
	}
	if out, isErr := run("ListMcpPrompts", `{"server":"srv"}`); isErr || !strings.Contains(out, "greet") || !strings.Contains(out, "[who]") {
		t.Fatal(out)
	}
	if out, isErr := run("GetMcpPrompt", `{"server":"srv","name":"greet"}`); isErr || !strings.Contains(out, "hi there") {
		t.Fatal(out)
	}
	if out, isErr := run("ReadMcpResource", `{"server":"nobody","uri":"x"}`); !isErr {
		t.Fatal(out)
	}
}

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
}

// pluginRepo builds a git repository holding a package of every kind of content.
func pluginRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range map[string]string{
		"plugin.json":            `{"name":"demo","version":"1.0.0","description":"a demo"}`,
		"commands/hello.md":      "---\ndescription: hello\n---\nSay hello\n",
		"skills/greet/SKILL.md":  "---\nname: greet\ndescription: greets\n---\nGreet.\n",
		"agents/helper.md":       "---\nname: helper\ndescription: helps\ntools: Read\n---\nHelp.\n",
		"output-styles/terse.md": "---\nname: terse\n---\nBe terse.\n",
		"hooks.json":             `{"hooks":{"PreCompact":[{"hooks":[{"type":"command","command":"cat \"${INFUSE_PLUGIN_ROOT}/plugin.json\" > fired.txt"}]}]}}`,
		".mcp.json":              `{"mcpServers":{"demo-srv":{"command":"demo-server"}}}`,
		"scripts/hook.sh":        "echo hi\n",
	} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-qm", "plugin")
	return "file://" + filepath.ToSlash(dir)
}

func TestPluginNeedsItsApprovalBeforeAnythingRuns(t *testing.T) {
	testHome(t)
	root := config.CanonRoot(t.TempDir())
	url := pluginRepo(t)
	p, err := InstallPlugin(context.Background(), root, url)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "demo" || p.Approved || p.Commit == "" || len(p.Runs) != 2 || exists(filepath.Join(p.Dir, ".git")) {
		t.Fatalf("%+v", p)
	}
	if _, err := InstallPlugin(context.Background(), root, url); err == nil {
		t.Fatal("installed over itself")
	}
	names := strings.Join(contentNames(root), " ")
	for _, want := range []string{"greet", "helper", "terse"} {
		if !strings.Contains(names, want) {
			t.Fatal(want, names)
		}
	}
	hooks := LoadHooks(root)
	if len(hooks) != 1 || hooks[0].Approved || !strings.Contains(hooks[0].Command, "/.infuse/plugins/demo/plugin.json") {
		t.Fatalf("%+v", hooks)
	}
	if err := approveHook(hooks[0].Key); err != nil {
		t.Fatal(err)
	}
	if LoadHooks(root)[0].Approved {
		t.Fatal("a plugin's hook was approved on its own")
	}
	if entries := mcp.For(root).Entries(); len(entries) != 0 {
		t.Fatalf("a server of an unapproved plugin: %+v", entries)
	}
	if err := ApprovePlugin(root, "demo"); err != nil {
		t.Fatal(err)
	}
	hooks = LoadHooks(root)
	entries := mcp.For(root).Entries()
	if !hooks[0].Approved || len(entries) != 1 || entries[0].Scope != "plugin:demo" || !mcp.Approved(root, entries[0].Scope, entries[0].Name, entries[0].Cfg) {
		t.Fatalf("%+v %+v", hooks, entries)
	}
	// a change in any file of the tree takes the approval away
	if err := os.WriteFile(filepath.Join(p.Dir, "scripts", "hook.sh"), []byte("echo changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if PluginApproved(root, "demo") || LoadHooks(root)[0].Approved || len(mcp.For(root).Entries()) != 0 {
		t.Fatal("the approval outlived the content it covered")
	}
	if err := RemovePlugin(root, "demo"); err != nil {
		t.Fatal(err)
	}
	if len(ListPlugins(root)) != 0 || exists(p.Dir) {
		t.Fatal("not removed")
	}
	if err := RemovePlugin(root, "../settings"); err == nil {
		t.Fatal("a name walked out of the folder")
	}
}

func contentNames(root string) []string {
	var out []string
	for _, sk := range LoadSkills(root) {
		out = append(out, sk.Name)
	}
	for _, k := range loadUserKinds(root) {
		out = append(out, k.Name)
	}
	for _, st := range OutputStyles(root) {
		if st.Source != "builtin" {
			out = append(out, st.Name)
		}
	}
	return out
}

func TestPluginRefusesBadSources(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	for _, u := range []string{"--upload-pack=evil", "ext::sh -c evil", "/etc/passwd", ""} {
		if _, err := InstallPlugin(context.Background(), root, u); err == nil {
			t.Fatal("installed from", u)
		}
	}
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"../evil"}`), 0o644)
	gitIn(t, dir, "init", "-q")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-qm", "x")
	if _, err := InstallPlugin(context.Background(), root, "file://"+filepath.ToSlash(dir)); err == nil {
		t.Fatal("a manifest name walked out of the folder")
	}
	entries, _ := os.ReadDir(pluginsDir(root))
	if len(entries) != 0 {
		t.Fatalf("a failed install left %v", entries)
	}
}

func TestMarketIndex(t *testing.T) {
	for _, doc := range []string{`[{"name":"a","url":"https://x"}]`, `{"plugins":[{"name":"a","url":"https://x"}]}`} {
		list, err := ParseMarketIndex([]byte(doc))
		if err != nil || len(list) != 1 || list[0].Name != "a" {
			t.Fatal(doc, err, list)
		}
	}
	if _, err := ParseMarketIndex([]byte("nope")); err == nil {
		t.Fatal("not JSON")
	}
}

func TestImpasseOutsideAMission(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Turn = 2
	s.Tasks = []TaskItem{{Text: "open", Status: "pending"}}
	var shown []string
	ch, stop := s.subscribe()
	defer stop()
	out, isErr := impasseTool{}.Run(context.Background(), &ToolContext{S: s, Root: root, Cwd: root}, json.RawMessage(`{"reason":"no database","tried":["connect","read docs"],"evidence":"refused","needs":"the URL"}`))
	if isErr || !strings.Contains(out, "the turn ends here") {
		t.Fatal(out)
	}
	for len(ch) > 0 {
		ev := <-ch
		b, _ := json.Marshal(ev.Data)
		shown = append(shown, string(b))
	}
	text := strings.Join(shown, " ")
	if !strings.Contains(text, "no database") || !strings.Contains(text, "the URL") {
		t.Fatal("the user must see it:", text)
	}
	if s.Outcome != "impasse" || s.Impasse == nil || s.Impasse.Needs != "the URL" || s.Tasks[0].Status == "pending" {
		t.Fatalf("%s %+v %+v", s.Outcome, s.Impasse, s.Tasks)
	}
	if !s.impassed() {
		t.Fatal("the turn must end")
	}
	if !s.endTurn(context.Background(), &runState{}) {
		t.Fatal("an impasse owes no further obligations")
	}
	s.Turn = 3
	if s.impassed() {
		t.Fatal("an impasse ends its own turn only")
	}
}

func TestFolderInstructionsLoadWhenTheShellEnters(t *testing.T) {
	testHome(t)
	wd, _ := os.Getwd()
	root, err := os.MkdirTemp(wd, "shell-cd-") // on a drive path: the shell's temp folder has another spelling
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	sub := filepath.Join(root, "pkg", "inner")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(root, "pkg", "CLAUDE.md"), []byte("rule of pkg"), 0o644)
	_ = os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("rule of inner"), 0o644)
	s := newSession(root)
	s.Mode = "bypass"
	s.settings.ContainProcesses = new(bool)
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{ID: "1", Name: "Bash", Input: bashInputOf("cd pkg/inner && pwd")})
	if isErr || !strings.Contains(out, "rule of pkg") || !strings.Contains(out, "rule of inner") {
		t.Fatalf("%v %s", isErr, out)
	}
	again, _ := s.runTool(context.Background(), &RuleSet{}, Block{ID: "2", Name: "Bash", Input: bashInputOf("pwd")})
	if strings.Contains(again, "rule of inner") {
		t.Fatal("once per session:", again)
	}
}
