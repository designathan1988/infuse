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

func TestBackgroundProcesses(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	s.gate.diagnosed = true // this test is about another gate
	s.Mode = "bypass"
	run := func(name, in string) (string, bool) {
		return s.runTool(context.Background(), &RuleSet{}, Block{Type: "tool_use", ID: "x", Name: name, Input: json.RawMessage(in)})
	}
	// returns at once with the id and the first output, while the command goes on
	start := time.Now()
	out, isErr := run("Bash", `{"command":"echo pronto; sleep 3; echo fim","run_in_background":true,"description":"servidor de teste"}`)
	if isErr || !strings.Contains(out, "p1") || !strings.Contains(out, "pronto") || time.Since(start) > 2800*time.Millisecond {
		t.Fatalf("%v %s", time.Since(start), out)
	}
	if ps := s.procList(); len(ps) != 1 || ps[0]["description"] != "servidor de teste" {
		t.Fatalf("%+v", ps)
	}
	for i := 0; i < 100 && s.proc("p1").State == "running"; i++ {
		time.Sleep(100 * time.Millisecond)
	}
	if st := s.proc("p1").State; st != "exited" {
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
	if out, _ = run("BashOutput", `{"id":"p1"}`); !strings.Contains(out, "fim") || strings.Contains(out, "pronto") || !strings.Contains(out, "exited") {
		t.Fatalf("new output: %s", out)
	}
	if out, _ = run("BashOutput", `{"id":"p1"}`); !strings.Contains(out, "no new output") {
		t.Fatal(out)
	}
	// stopping ends the whole tree: the shell and the program it started
	out, _ = run("Bash", `{"command":"sleep 60 & echo filho $!; wait","run_in_background":true}`)
	m := regexp.MustCompile(`filho (\d+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("child pid: %s", out)
	}
	sleeps := func() string {
		if runtime.GOOS != "windows" {
			return ""
		}
		ps, err := exec.Command("powershell", "-NoProfile", "-Command", `@(Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'sleep.exe' -and $_.CommandLine -like '*sleep*60*' }).Count`).CombinedOutput()
		if err != nil {
			t.Fatalf("process query: %v %s", err, ps)
		}
		return strings.TrimSpace(string(ps))
	}
	if runtime.GOOS == "windows" && sleeps() != "1" {
		txt, _ := s.proc("p2").out.since(0)
		t.Fatalf("the child should be running before the stop: %q (p2 %s: %q)", sleeps(), s.proc("p2").State, txt)
	}
	if out, isErr = run("KillShell", `{"id":"p2"}`); isErr {
		t.Fatal(out)
	}
	time.Sleep(800 * time.Millisecond)
	if p := s.proc("p2"); p.State != "stopped" {
		t.Fatalf("state %s", p.State)
	}
	// no process of the tree is left: the program the shell started ended with it
	if runtime.GOOS == "windows" {
		if n := sleeps(); n != "" && n != "0" {
			t.Fatalf("%s process(es) of the stopped tree still running", n)
		}
	} else if child, _ := strconv.Atoi(m[1]); processAlive(child) {
		t.Fatal("child still running")
	}
	if out, _ = run("BashOutput", `{"id":"p9"}`); !strings.Contains(out, "p1") {
		t.Fatalf("unknown id should list the session's processes: %s", out)
	}
}

func TestParallelSubagentsAreEachRecorded(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.gate.diagnosed = true // this test is about another gate
	s.Provider = "mock"
	content := []Block{
		{Type: "tool_use", ID: "a", Name: "Agent", Input: json.RawMessage(`{"kind":"explore","description":"pesquisa A","prompt":"olá 1"}`)},
		{Type: "tool_use", ID: "b", Name: "Agent", Input: json.RawMessage(`{"kind":"explore","description":"pesquisa B","prompt":"olá 2"}`)},
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
	if !titles["pesquisa A"] || !titles["pesquisa B"] {
		t.Fatalf("%+v", s.Agents)
	}
	if snap := s.snapshot(); len(snap["agents"].([]AgentRun)) != 2 {
		t.Fatal("agents missing from the snapshot")
	}
}
