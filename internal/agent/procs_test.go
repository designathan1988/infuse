package agent

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type procTestEnv struct {
	t   *testing.T
	s   *Session
	run func(name, in string) (string, bool)
}

func newProcTestEnv(t *testing.T) *procTestEnv {
	t.Helper()
	testHome(t)
	s := newSession(t.TempDir())
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	return &procTestEnv{t: t, s: s, run: func(name, in string) (string, bool) {
		return s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "x", Name: name, Input: json.RawMessage(in)})
	}}
}

func TestBackgroundProcesses(t *testing.T) {
	e := newProcTestEnv(t)
	e.startAndFinish()
	e.killTree()
	if out, _ := e.run("BashOutput", `{"id":"p9"}`); !strings.Contains(out, "p1") {
		t.Fatalf("unknown id should list the session's processes: %s", out)
	}
}

func (e *procTestEnv) startAndFinish() {
	t, s, run := e.t, e.s, e.run
	// returns at once with the id and the first output, while the command goes on
	start := time.Now()
	out, isErr := run("Bash", `{"command":"echo ready; sleep 3; echo end","run_in_background":true,"description":"test server"}`)
	if isErr || !strings.Contains(out, "p1") || !strings.Contains(out, "ready") || time.Since(start) > 2800*time.Millisecond {
		t.Fatalf("%v %s", time.Since(start), out)
	}
	if ps := s.procList(); len(ps) != 1 || ps[0]["description"] != "test server" {
		t.Fatalf("%+v", ps)
	}
	for i := 0; i < 100 && e.procState("p1") == "running"; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if st := e.procState("p1"); st != "exited" {
		t.Fatalf("state %s", st)
	}
	// the agent is told it ended, once, on its next tool result
	out, _ = run("Glob", `{"pattern":"*"}`)
	if !strings.Contains(out, "p1") || !strings.Contains(out, "finished on its own") {
		t.Fatalf("end not announced: %s", out)
	}
	if out, _ = run("Glob", `{"pattern":"*"}`); strings.Contains(out, "finished on its own") {
		t.Fatal("announced twice")
	}
	// only the new output, then nothing new
	if out, _ = run("BashOutput", `{"id":"p1"}`); !strings.Contains(out, "end") || strings.Contains(out, "ready") || !strings.Contains(out, "exited") {
		t.Fatalf("new output: %s", out)
	}
	if out, _ = run("BashOutput", `{"id":"p1"}`); !strings.Contains(out, "no new output") {
		t.Fatal(out)
	}
}

func (e *procTestEnv) killTree() {
	t, s, run := e.t, e.s, e.run
	// stopping ends the whole tree: the shell and the program it started
	out, _ := run("Bash", `{"command":"sleep 60 & echo child $!; wait","run_in_background":true}`)
	m := regexp.MustCompile(`child (\d+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("child pid: %s", out)
	}
	if runtime.GOOS == "windows" && e.sleeps() != "1" {
		txt, _ := s.proc("p2").out.since(0)
		t.Fatalf("the child should be running before the stop: %q (p2 %s: %q)", e.sleeps(), e.procState("p2"), txt)
	}
	if out, isErr := run("KillShell", `{"id":"p2"}`); isErr {
		t.Fatal(out)
	}
	time.Sleep(800 * time.Millisecond)
	if st := e.procState("p2"); st != "stopped" {
		t.Fatalf("state %s", st)
	}
	// no process of the tree is left: the program the shell started ended with it
	if runtime.GOOS == "windows" {
		if n := e.sleeps(); n != "" && n != "0" {
			t.Fatalf("%s process(es) of the stopped tree still running", n)
		}
	} else if child, _ := strconv.Atoi(m[1]); processAlive(child) {
		t.Fatal("child still running")
	}
}

func (e *procTestEnv) procState(id string) string {
	e.s.Mu.Lock()
	defer e.s.Mu.Unlock()
	for _, p := range e.s.procs {
		if p.ID == id {
			return p.State
		}
	}
	return ""
}

func (e *procTestEnv) sleeps() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	ps, err := exec.Command("powershell", "-NoProfile", "-Command", `@(Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'sleep.exe' -and $_.CommandLine -like '*sleep*60*' }).Count`).CombinedOutput()
	if err != nil {
		e.t.Fatalf("process query: %v %s", err, ps)
	}
	return strings.TrimSpace(string(ps))
}

func TestParallelSubagentsAreEachRecorded(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.gate.diagnosed = true // this test is about another gate
	s.Provider = "mock"
	content := []Block{
		{Type: "tool_use", ID: "a", Name: "Agent", Input: json.RawMessage(`{"kind":"explore","description":"research A","prompt":"hello 1"}`)},
		{Type: "tool_use", ID: "b", Name: "Agent", Input: json.RawMessage(`{"kind":"explore","description":"research B","prompt":"hello 2"}`)},
	}
	_ = s.runReadsInParallel(context.Background(), &RuleSet{}, content)
	if len(s.Agents) != 2 {
		t.Fatalf("%+v", s.Agents)
	}
	titles := map[string]bool{}
	for _, a := range s.Agents {
		titles[a.Title] = true
		if a.State != "done" || a.Result == "" || a.Ended == 0 || a.Kind != "explore" {
			t.Fatalf("%+v", a)
		}
	}
	if !titles["research A"] || !titles["research B"] {
		t.Fatalf("%+v", s.Agents)
	}
	if snap := s.snapshot(); len(snap["agents"].([]AgentRun)) != 2 {
		t.Fatal("agents missing from the snapshot")
	}
}

// A dev server's address printed by a background command reaches the Browser pane, once.
func TestProcessOutputSurfacesTheDevServer(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	ch, unsub := s.Subscribe()
	defer unsub()
	w := &procURLWatcher{log: &procLog{}, s: s, seen: map[string]bool{}}
	if _, err := w.Write([]byte("VITE ready\n  Local: http://localhost:5173/\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-ch:
		d, _ := json.Marshal(ev.Data)
		if ev.Type != "browser" || !strings.Contains(string(d), "http://localhost:5173/") {
			t.Fatalf("event %s %s", ev.Type, d)
		}
	case <-time.After(time.Second):
		t.Fatal("the address never reached the session")
	}
	if _, _ = w.Write([]byte("  Local: http://localhost:5173/\n")); true {
		select {
		case ev := <-ch:
			t.Fatalf("the same address was emitted twice: %+v", ev)
		case <-time.After(100 * time.Millisecond):
		}
	}
}
