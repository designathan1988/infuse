package agent

import (
	"context"
	"encoding/json"
	"infuse/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func bashInputOf(cmd string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"command": cmd})
	return b
}

// "Always" on one command remembers its prefix, not the whole of Bash.
func TestAlwaysRemembersThePrefixOnly(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	stop := answerPermissions(s, "always")
	ok := s.askPermission(context.Background(), bashTool{}, bashInputOf("git status --short"))
	stop()
	if !ok {
		t.Fatal("the answer was yes")
	}
	for cmd, want := range map[string]string{
		"git status":                 "allow",
		"git status --porcelain":     "allow",
		"git push":                   "ask",
		"rm notes.txt":               "ask",
		"git status && rm notes.txt": "ask",
		"git status $(rm x)":         "ask",
		"git status `rm x`":          "ask",
	} {
		if got := s.permissionFor(bashTool{}, bashInputOf(cmd)); got != want {
			t.Errorf("%q: %s, want %s", cmd, got, want)
		}
	}
	stop = answerPermissions(s, "always")
	_ = s.askPermission(context.Background(), webFetchTool{}, json.RawMessage(`{"url":"https://a.dev"}`))
	stop()
	if got := s.permissionFor(webFetchTool{}, json.RawMessage(`{"url":"https://b.dev"}`)); got != "allow" {
		t.Fatal("always on another tool stays the whole tool:", got)
	}
}

func TestThePermissionQuestionNamesThePrefix(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	done := make(chan struct{})
	go func() { s.askPermission(context.Background(), bashTool{}, bashInputOf("ls -la && pwd")); close(done) }()
	var prefix string
	for prefix == "" {
		s.Mu.Lock()
		if s.pendingPerm != nil {
			prefix = s.pendingPerm.Prefix
		}
		s.Mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	s.ResolvePermission(s.pendingPerm.ID, "deny")
	<-done
	if prefix != "ls" {
		t.Fatal(prefix)
	}
}

func TestPatternRules(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.settings.Permissions = map[string]string{
		"Bash":                "ask",
		"Bash(git status:*)":  "allow",
		"Bash(npm test)":      "allow",
		"Bash(git push:*)":    "deny",
		"Bash(curl:*)":        "ask",
		"Write(src/**)":       "allow",
		"Write(src/secret/*)": "deny",
		"Edit(docs/**)":       "ask",
		"WebFetch(*.go.dev)":  "allow",
	}
	s.settings.Permissions["Read(private/**)"] = "deny"
	cases := []struct {
		tool Tool
		in   string
		want string
	}{
		{bashTool{}, `{"command":"git status -s"}`, "allow"},
		{bashTool{}, `{"command":"git statusx"}`, "ask"},
		{bashTool{}, `{"command":"npm test"}`, "allow"},
		{bashTool{}, `{"command":"npm test --force"}`, "ask"},
		{bashTool{}, `{"command":"git status; git push origin"}`, "deny"},
		{bashTool{}, `{"command":"git status && curl x"}`, "ask"},
		{bashTool{}, `{"command":"git status && ls"}`, "ask"},
		{writeTool{}, `{"path":"src/a/b.go","content":"x"}`, "allow"},
		{writeTool{}, `{"path":"src/secret/key.go","content":"x"}`, "deny"},
		{writeTool{}, `{"path":"other/b.go","content":"x"}`, "ask"},
		{webFetchTool{}, `{"url":"https://pkg.go.dev/net/http"}`, "allow"},
		{webFetchTool{}, `{"url":"https://example.com"}`, "ask"},
		{readTool{}, `{"path":"private/x.txt"}`, "deny"},
		{readTool{}, `{"path":"public/x.txt"}`, "allow"},
	}
	for _, c := range cases {
		if got := s.permissionFor(c.tool, json.RawMessage(c.in)); got != c.want {
			t.Errorf("%s %s: %s, want %s", c.tool.Spec().Name, c.in, got, c.want)
		}
	}
	s.Mode = "bypass"
	if got := s.permissionFor(bashTool{}, bashInputOf("git push")); got != "deny" {
		t.Fatal("a deny rule holds in bypass:", got)
	}
	if got := s.permissionFor(bashTool{}, bashInputOf("curl x")); got != "allow" {
		t.Fatal("bypass asks nothing a deny rule does not stop:", got)
	}
	s.Mode = "plan"
	if got := s.permissionFor(bashTool{}, bashInputOf("git status")); got != "deny" {
		t.Fatal("an allow rule does not open plan mode:", got)
	}
}

func TestDestructiveTable(t *testing.T) {
	hits := []string{
		"rm -rf build", "rm -fr build", "rm -r -f build", "cd x && rm -Rf y", "sudo rm --recursive --force z",
		"git push --force", "git push origin main -f", "git push origin +main", "git push --force-with-lease",
		"git reset --hard HEAD~1", "git -C sub reset --hard", "git clean -fd", "git clean -xfd",
		`psql -c "DROP TABLE users"`, "echo 'drop database x' | mysql", "Remove-Item -Recurse -Force build",
	}
	for _, c := range hits {
		if _, hit := destructiveMatch(c); !hit {
			t.Errorf("missed: %s", c)
		}
	}
	misses := []string{
		"rm file.txt", "rm -r build", "git push origin main", "git reset HEAD~1", "git reset --soft HEAD~1", "git clean -n",
		"ls -rf", "cat > notes.md <<'EOF'\nrm -rf /\ngit push --force\nEOF\n", "grep -r drop src",
	}
	for _, c := range misses {
		if r, hit := destructiveMatch(c); hit {
			t.Errorf("false alarm %s on: %s", r.ID, c)
		}
	}
}

func TestDestructiveAsksInEveryModeUnlessExplicitlyAllowed(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	_ = os.MkdirAll(victim, 0o755)
	s := newSession(root)
	s.Mode = "bypass"
	b := Block{Type: "tool_use", ID: "1", Name: "Bash", Input: bashInputOf("rm -rf victim")}
	stop := answerPermissions(s, "deny")
	decision, refusal, refused := s.destructiveGate(context.Background(), bashTool{}, b, "allow")
	stop()
	if !refused || !strings.Contains(refusal, "rm -rf") || decision != "allow" {
		t.Fatal("bypass ran a destructive command unasked:", refusal)
	}
	stop = answerPermissions(s, "always")
	decision, _, refused = s.destructiveGate(context.Background(), bashTool{}, b, "ask")
	stop()
	if refused || decision != "allow" {
		t.Fatal("approved:", decision)
	}
	if _, _, refused := s.destructiveGate(context.Background(), bashTool{}, b, "allow"); refused {
		t.Fatal("the same command, already approved for the session")
	}
	s.Mu.Lock()
	other := s.pendingPerm
	s.Mu.Unlock()
	if other != nil {
		t.Fatal("asked again")
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		_, _, _ = s.destructiveGate(ctx, bashTool{}, Block{ID: "2", Name: "Bash", Input: bashInputOf("rm -rf other")}, "allow")
	}()
	asked := false
	for i := 0; i < 200 && !asked; i++ {
		s.Mu.Lock()
		asked = s.pendingPerm != nil
		s.Mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	if !asked {
		t.Fatal("always must cover the exact command, never the whole pattern")
	}
	s.settings.Permissions["Bash(rm -rf build:*)"] = "allow"
	if _, _, refused := s.destructiveGate(context.Background(), bashTool{}, Block{ID: "3", Name: "Bash", Input: bashInputOf("rm -rf build")}, "allow"); refused {
		t.Fatal("the user's explicit allow")
	}
	if decision, _, _ := s.destructiveGate(context.Background(), bashTool{}, b, "deny"); decision != "deny" {
		t.Fatal("a deny stays")
	}
}

func TestSettingsEnvReachesBashAndHooksNotTheModel(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	s.settings.ContainProcesses, s.settings.RequireDiagnosis = new(bool), new(bool)
	s.settings.Env = map[string]string{"PROJECT_TOKEN": "sup3r-v4lue"}
	out, isErr := s.runTool(context.Background(), &RuleSet{}, Block{ID: "1", Name: "Bash", Input: bashInputOf(`test "$PROJECT_TOKEN" = sup3r-v4lue && echo seen=$PROJECT_TOKEN`)})
	if isErr || strings.Contains(out, "sup3r-v4lue") || !strings.Contains(out, "seen=[env:PROJECT_TOKEN]") {
		t.Fatalf("%v %q", isErr, out)
	}
	marker := filepath.ToSlash(filepath.Join(root, "hook-env"))
	if err := addHook(root, "project", "PreCompact", "", `echo "$PROJECT_TOKEN" > `+marker); err != nil {
		t.Fatal(err)
	}
	for _, e := range LoadHooks(root) {
		_ = approveHook(e.Key)
	}
	s.preCompactHooks(context.Background(), "auto")
	if b, _ := os.ReadFile(filepath.FromSlash(marker)); !strings.Contains(string(b), "sup3r-v4lue") {
		t.Fatalf("hook env: %q", b)
	}
	if strings.Contains(buildSystemPrompt(s, &RuleSet{}), "sup3r-v4lue") {
		t.Fatal("a variable reached the prompt")
	}
}

func TestOutputStyles(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	dir := filepath.Join(root, infuseDirName, "output-styles")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "pirate.md"), []byte("---\nname: pirate\ndescription: arr\n---\nWrite like a pirate.\n"), 0o644)
	s := newSession(root)
	names := map[string]bool{}
	for _, st := range OutputStyles(root) {
		names[st.Name] = true
	}
	for _, n := range []string{"default", "concise", "explanatory", "learning", "pirate"} {
		if !names[n] {
			t.Fatal("missing style", n)
		}
	}
	s.settings.OutputStyle = "pirate"
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), "Write like a pirate.") {
		t.Fatal("the style did not reach the prompt")
	}
	s.settings.OutputStyle = "concise"
	if !strings.Contains(buildSystemPrompt(s, &RuleSet{}), "as short as the facts allow") {
		t.Fatal("a catalog style did not reach the prompt")
	}
	s.settings.OutputStyle = "gone"
	if strings.Contains(buildSystemPrompt(s, &RuleSet{}), "## Output style") {
		t.Fatal("a style that does not exist")
	}
}

func TestStatusLine(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Model, s.Branch, s.Cost = "m1", "dev", 1.5
	s.settings.ContainProcesses = new(bool)
	if got := s.StatusLine(context.Background()); got != "m1 · default · dev" {
		t.Fatal(got)
	}
	s.settings.StatusLine = &config.StatusLine{Template: "{branch} ${cost}"}
	if got := s.StatusLine(context.Background()); got != "dev $1.50" {
		t.Fatal(got)
	}
	s.settings.StatusLine = &config.StatusLine{Command: `read j; echo "from-command"`, Template: "fallback"}
	if got := s.StatusLine(context.Background()); got != "from-command" {
		t.Fatal(got)
	}
	s.settings.StatusLine = &config.StatusLine{Command: "exit 3", Template: "fallback"}
	if got := s.StatusLine(context.Background()); got != "fallback" {
		t.Fatal(got)
	}
}

func TestTurnLogIsQueryableAndExports(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Turn = 3
	var spans atomic.Int32
	var body atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var doc map[string]any
		_ = json.NewDecoder(r.Body).Decode(&doc)
		if r.URL.Path == "/v1/traces" && r.Header.Get("Content-Type") == "application/json" && doc["resourceSpans"] != nil {
			b, _ := json.Marshal(doc)
			body.Store(string(b))
			spans.Add(1)
		}
	}))
	defer srv.Close()
	s.settings.OTLPEndpoint = srv.URL + "/"
	s.settings.Env = map[string]string{"PROJECT_TOKEN": "sup3r-v4lue"}
	req := ModelRequest{Model: "m", System: "secret system prompt", Messages: []Message{{}}, MaxTokens: 9}
	res := ModelResult{StopReason: "tool_use", Usage: Usage{Input: 11, Output: 7}, Content: []Block{{Type: "tool_use", ID: "t1", Name: "Bash", Input: bashInputOf("echo sup3r-v4lue")}}}
	s.logModelCall(req, res, time.Now().Add(-5*time.Millisecond))
	s.logToolCalls(res, []Block{{Type: "tool_result", ToolUseID: "t1", Content: "hello", Ms: 12}})
	all := QueryTurnLog(root, TurnQuery{Session: s.ID})
	if len(all) != 2 {
		t.Fatalf("%+v", all)
	}
	m := all[0]
	if m["v"] != float64(turnLogSchema) || m["kind"] != "model" || m["turn"] != float64(3) || m["model"] != "m" {
		t.Fatalf("%+v", m)
	}
	if u, _ := m["usage"].(map[string]any); u["input"] != float64(11) {
		t.Fatalf("%+v", m)
	}
	raw, _ := os.ReadFile(turnLogFile(root, s.ID))
	if strings.Contains(string(raw), "secret system prompt") || strings.Contains(string(raw), "sup3r-v4lue") {
		t.Fatalf("the log kept a prompt or a variable: %s", raw)
	}
	if got := QueryTurnLog(root, TurnQuery{Session: s.ID, Kind: "tool", Tool: "bash"}); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
	if got := QueryTurnLog(root, TurnQuery{Session: s.ID, Turn: 9}); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	if got := QueryTurnLog(root, TurnQuery{Session: `..\..\` + s.ID}); len(got) != 2 {
		t.Fatalf("a session id walks no folders: %d", len(got))
	}
	for i := 0; i < 500 && spans.Load() < 2; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	if spans.Load() != 2 {
		t.Fatal("spans:", spans.Load())
	}
	if text, _ := body.Load().(string); strings.Contains(text, "sup3r-v4lue") || strings.Contains(text, "echo") {
		t.Fatalf("a span carried content: %s", text)
	}
}

func TestTelemetryIsOffByDefault(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	before := len(otlpSlots)
	exportOTLP(s, TurnRecord{Kind: "model"})
	if len(otlpSlots) != before {
		t.Fatal("an export without an endpoint")
	}
}
