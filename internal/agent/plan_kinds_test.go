package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func answerPermissions(s *Session, decision string) (stop func()) {
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(5 * time.Millisecond):
			}
			s.Mu.Lock()
			req := s.pendingPerm
			s.Mu.Unlock()
			if req != nil {
				s.ResolvePermission(req.ID, decision)
			}
		}
	}()
	return func() { close(done) }
}

func TestPlanModeToolsNeedTheUserToLeave(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "acceptEdits"
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}
	if out, _ := (enterPlanModeTool{}).Run(context.Background(), tc, nil); !strings.Contains(out, "plan mode on") || s.mode() != "plan" {
		t.Fatal(out, s.mode())
	}
	if got := s.permissionFor(writeTool{}, json.RawMessage(`{}`)); got != "deny" {
		t.Fatal("plan mode allowed a write:", got)
	}
	if got := s.permissionFor(exitPlanModeTool{}, json.RawMessage(`{}`)); got != "allow" {
		t.Fatal("the tool that asks must be reachable in plan mode:", got)
	}
	stop := answerPermissions(s, "deny")
	out, isErr := exitPlanModeTool{}.Run(context.Background(), tc, json.RawMessage(`{"plan":"step one"}`))
	stop()
	if isErr || s.mode() != "plan" || s.Plan != "step one" {
		t.Fatal("a refusal must keep plan mode:", out, s.mode())
	}
	stop = answerPermissions(s, "allow")
	out, isErr = exitPlanModeTool{}.Run(context.Background(), tc, json.RawMessage(`{"plan":"step one"}`))
	stop()
	if isErr || s.mode() != "acceptEdits" {
		t.Fatal("approval must restore the previous mode:", out, s.mode())
	}
	if out, isErr := (exitPlanModeTool{}).Run(context.Background(), tc, json.RawMessage(`{}`)); !isErr {
		t.Fatal("leaving plan mode outside it:", out)
	}
	reloaded := SessionData{}
	if err := readJSON(filepath.Join(sessionsDir(s.Root), s.ID+".json"), &reloaded); err != nil || reloaded.Mode != "acceptEdits" {
		t.Fatal("the mode did not persist:", err, reloaded.Mode)
	}
}

func TestSubagentsCannotTouchThePlanMode(t *testing.T) {
	if subagentAllows("general", "ExitPlanMode") || subagentAllows("general", "EnterPlanMode") || subagentAllows("general", "Agent") {
		t.Fatal("a subagent reached the session's controls")
	}
}

func TestUserSubagentKinds(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	dir := filepath.Join(root, infuseDirName, "agents")
	_ = os.MkdirAll(dir, 0o755)
	def := "---\nname: docs-writer\ndescription: writes the docs, with care\ntools: Read, Grep\nmodel: fast\n---\nYou write documentation.\n"
	listed := "---\ndescription: lists\ntools:\n  - Read\n  - Write\n---\nRole.\n"
	takeover := "---\nname: general\n---\nTaken over.\n"
	_ = os.WriteFile(filepath.Join(dir, "docs.md"), []byte(def), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "lister.md"), []byte(listed), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "takeover.md"), []byte(takeover), 0o644)
	s := newSession(root)
	k, ok := s.subagentKind("docs-writer")
	if !ok || k.Description != "writes the docs, with care" || !k.ReadOnly || k.Model != "fast" {
		t.Fatalf("%+v", k)
	}
	if !k.allows("Grep") || k.allows("Write") || k.allows("Bash") || k.allows("Agent") {
		t.Fatalf("restricted tools leaked: %+v", k)
	}
	if !strings.Contains(subagentSystem(s, k), "You write documentation.") {
		t.Fatal("the kind's body is its role")
	}
	l, ok := s.subagentKind("lister")
	if !ok || l.ReadOnly || !l.allows("Write") || l.allows("Bash") {
		t.Fatalf("%+v", l)
	}
	if g, _ := s.subagentKind("general"); !strings.Contains(g.Role, "bounded task") {
		t.Fatal("a file took over a built-in kind:", g.Role)
	}
	if _, ok := s.subagentKind("nobody"); ok {
		t.Fatal("unknown kind")
	}
	var agentSpec ToolSpec
	for _, sp := range s.toolSpecs() {
		if sp.Name == "Agent" {
			agentSpec = sp
		}
	}
	if !strings.Contains(string(agentSpec.InputSchema), "docs-writer") || !strings.Contains(agentSpec.Description, "kind=docs-writer") {
		t.Fatalf("%s", agentSpec.InputSchema)
	}
	if _, err := s.RunSubagent(context.Background(), "nobody", "", "x"); err == nil {
		t.Fatal("an unknown kind ran")
	}
}

func TestNewHookEventsFire(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	marker := filepath.ToSlash(filepath.Join(root, "fired"))
	for _, ev := range []string{"SessionStart", "PreCompact", "Notification"} {
		if err := addHook(root, "project", ev, "", "echo "+ev+" >> "+marker+"; echo context-from-"+ev); err != nil {
			t.Fatal(err)
		}
	}
	entries := LoadHooks(root)
	if len(entries) != 3 {
		t.Fatalf("%+v", entries)
	}
	for _, e := range entries {
		if err := approveHook(e.Key); err != nil {
			t.Fatal(err)
		}
	}
	s := newSession(root)
	s.settings.ContainProcesses = new(bool)
	s.sessionStartHooks(context.Background())
	s.sessionStartHooks(context.Background()) // once per open session
	if !strings.Contains(s.sessionStartContext(), "context-from-SessionStart") {
		t.Fatalf("%q", s.sessionStartContext())
	}
	s.preCompactHooks(context.Background(), "auto")
	s.notificationHooks("permission_prompt", "Bash")
	deadline := time.Now().Add(20 * time.Second)
	var got string
	for time.Now().Before(deadline) {
		b, _ := os.ReadFile(filepath.FromSlash(marker))
		got = string(b)
		if strings.Contains(got, "Notification") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if strings.Count(got, "SessionStart") != 1 || !strings.Contains(got, "PreCompact") || !strings.Contains(got, "Notification") {
		t.Fatalf("%q", got)
	}
}

// slowReadTool lets the test see two calls of one group inside the tool at the same time.
type slowReadTool struct {
	name    string
	cat     string
	running *atomic.Int32
	peak    *atomic.Int32
}

func (t slowReadTool) Category() string { return t.cat }
func (t slowReadTool) Spec() ToolSpec   { return ToolSpec{Name: t.name} }
func (t slowReadTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	n := t.running.Add(1)
	if n > t.peak.Load() {
		t.peak.Store(n)
	}
	time.Sleep(60 * time.Millisecond)
	t.running.Add(-1)
	return "ok", false
}

func TestGroupsKeepTheOrderOfTheReply(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	content := []Block{
		{Type: "text", Text: "first"},
		{Type: "tool_use", ID: "1", Name: "Read", Input: json.RawMessage(`{"path":"a.txt"}`)},
		{Type: "tool_use", ID: "2", Name: "Read", Input: json.RawMessage(`{"path":"a.txt"}`)},
		{Type: "tool_use", ID: "3", Name: "Write", Input: json.RawMessage(`{"path":"a.txt","content":"x"}`)},
		{Type: "tool_use", ID: "4", Name: "Read", Input: json.RawMessage(`{"path":"a.txt"}`)},
		{Type: "tool_use", ID: "5", Name: "Grep", Input: json.RawMessage(`{"pattern":"x"}`)},
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("before"), 0o644)
	got := s.runParallelFrom(context.Background(), &RuleSet{}, content, 1)
	if len(got) != 2 || got["3"].ToolUseID != "" || got["4"].ToolUseID != "" {
		t.Fatalf("a read after a write ran before it: %+v", got)
	}
	if later := s.runParallelFrom(context.Background(), &RuleSet{}, content, 3); len(later) != 0 {
		t.Fatalf("a group cannot start at a write: %+v", later)
	}
	if last := s.runParallelFrom(context.Background(), &RuleSet{}, content, 4); len(last) != 2 {
		t.Fatalf("%+v", last)
	}
}

func TestNetworkCallsJoinAGroupOnlyWhenTheyNeedNoQuestion(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	fetch := Block{Type: "tool_use", ID: "1", Name: "WebFetch", Input: json.RawMessage(`{"url":"https://example.com"}`)}
	if s.parallelSafe(fetch) {
		t.Fatal("a call that asks ran in a group")
	}
	s.Mode = "bypass"
	if !s.parallelSafe(fetch) {
		t.Fatal("an allowed fetch is independent")
	}
	if s.parallelSafe(Block{Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)}) {
		t.Fatal("a shell command is never independent")
	}
	if s.parallelSafe(Block{Name: "Agent", Input: json.RawMessage(`{"kind":"general"}`)}) || !s.parallelSafe(Block{Name: "Agent", Input: json.RawMessage(`{"kind":"explore"}`)}) {
		t.Fatal("subagents: read-only kinds only")
	}
}

func TestSubagentReadsRunSideBySide(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.Mode = "bypass"
	var running, peak atomic.Int32
	toolRegistry["SlowRead"] = slowReadTool{name: "SlowRead", cat: "read", running: &running, peak: &peak}
	defer delete(toolRegistry, "SlowRead")
	content := []Block{
		{Type: "tool_use", ID: "1", Name: "SlowRead", Input: json.RawMessage(`{}`)},
		{Type: "tool_use", ID: "2", Name: "SlowRead", Input: json.RawMessage(`{}`)},
	}
	run := s.agentStart("a1", "explore", "t", "t")
	allowed := func(string) bool { return true }
	results, err := s.subagentToolTurn(context.Background(), &RuleSet{}, allowed, content, run, "a1", &loopBreaker{})
	if err != nil || len(results) != 2 || results[0].ToolUseID != "1" || results[1].ToolUseID != "2" {
		t.Fatalf("%v %+v", err, results)
	}
	if peak.Load() != 2 {
		t.Fatal("the two reads did not overlap:", peak.Load())
	}
}
