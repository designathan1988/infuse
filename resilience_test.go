package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptProvider answers each request with the next step of a script; the last step repeats.
type scriptProvider struct {
	mu    sync.Mutex
	steps []func(req ModelRequest) (ModelResult, error)
	n     int
}

func (p *scriptProvider) Name() string { return "script" }
func (p *scriptProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	p.mu.Lock()
	i := min(p.n, len(p.steps)-1)
	p.n++
	p.mu.Unlock()
	return p.steps[i](req)
}

func say(text string) func(ModelRequest) (ModelResult, error) {
	return func(ModelRequest) (ModelResult, error) {
		return ModelResult{Content: []Block{{Type: "text", Text: text}}, StopReason: "end_turn"}, nil
	}
}

func call(name, input string) func(ModelRequest) (ModelResult, error) {
	return func(ModelRequest) (ModelResult, error) {
		return ModelResult{Content: []Block{{Type: "tool_use", ID: fmt.Sprintf("c%d", time.Now().UnixNano()), Name: name, Input: json.RawMessage(input)}}, StopReason: "tool_use"}, nil
	}
}

func fails(err error) func(ModelRequest) (ModelResult, error) {
	return func(ModelRequest) (ModelResult, error) { return ModelResult{}, err }
}

var scriptSeq int

// runScript runs one turn of a session against the script and waits for it to end.
func runScript(t *testing.T, steps ...func(ModelRequest) (ModelResult, error)) (*Session, *scriptProvider) {
	t.Helper()
	testHome(t)
	saved := retryWaits
	retryWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryWaits = saved })
	scriptSeq++
	name := fmt.Sprintf("script-%d", scriptSeq)
	p := &scriptProvider{steps: steps}
	registerProvider(name, func(ProviderConfig) Provider { return p })
	// these tests script every model call: no extra call to compile prohibitions (read at Send)
	_ = writeJSON(settingsFile(), map[string]any{"userProhibitions": false})
	s := newSession(t.TempDir())
	s.Provider, s.Model, s.Mode = name, "script", "bypass"
	if err := s.Send("faça a tarefa", nil, nil); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	off := false
	s.settings.RequireReview = &off
	s.mu.Unlock()
	waitTurn(t, s)
	return s, p
}

func waitTurn(t *testing.T, s *Session) {
	t.Helper()
	for i := 0; i < 1500; i++ {
		s.mu.Lock()
		st, working := s.State, s.Working
		s.mu.Unlock()
		if !working && (st == "idle" || st == "error") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the turn did not end")
}

func transcript(s *Session) string {
	b, _ := json.Marshal(s.Messages)
	return string(b)
}

func TestProviderFailuresDoNotEndTheTurn(t *testing.T) {
	s, _ := runScript(t,
		fails(errors.New("DeepSeek 503: service overloaded")),
		fails(errors.New("read tcp: connection reset by peer")),
		func(ModelRequest) (ModelResult, error) { return ModelResult{}, nil }, // an empty answer
		say("pronto"))
	if s.State != "idle" || !strings.Contains(transcript(s), "pronto") {
		t.Fatalf("state %s, error %q", s.State, s.lastError)
	}
	// a key or request problem is not retried: the user must act
	s2, p2 := runScript(t, fails(errors.New("DeepSeek 401: invalid api key")), say("nunca"))
	if s2.State != "error" || p2.n != 1 {
		t.Fatalf("fatal error retried: state %s after %d calls", s2.State, p2.n)
	}
}

func TestContextTooLongCompactsAndGoesOn(t *testing.T) {
	var asked []int
	steps := []func(ModelRequest) (ModelResult, error){
		call("Glob", `{"pattern":"*"}`),
		call("Glob", `{"pattern":"*.go"}`),
		func(req ModelRequest) (ModelResult, error) {
			asked = append(asked, len(req.Messages))
			return ModelResult{}, errors.New("DeepSeek 400: This model's maximum context length is 1000000 tokens")
		},
		say("resumo"), // the compaction's summary
		say("continuando"),
	}
	s, _ := runScript(t, steps...)
	if s.State != "idle" || s.Compactions < 1 || !strings.Contains(transcript(s), "continuando") {
		t.Fatalf("state %s, compactions %d: %s", s.State, s.Compactions, s.lastError)
	}
}

func TestReplyCutAtTheLimitIsContinuedNotRun(t *testing.T) {
	ran := false
	registerTool(panicTool{&ran})
	cut := func(ModelRequest) (ModelResult, error) {
		return ModelResult{Content: []Block{{Type: "text", Text: "Vou escrever"}, {Type: "tool_use", ID: "half", Name: "PanicProbe", Input: json.RawMessage(`{"path":"a.ts","content":"exp`)}}, StopReason: "length"}, nil
	}
	s, _ := runScript(t, cut, say("terminei"))
	if ran {
		t.Fatal("a half-written call was executed")
	}
	if s.State != "idle" || !strings.Contains(transcript(s), "cut at the output limit") {
		t.Fatalf("state %s: %s", s.State, transcript(s))
	}
}

type panicTool struct{ ran *bool }

func (panicTool) Category() string { return "read" }
func (panicTool) Spec() ToolSpec {
	return ToolSpec{Name: "PanicProbe", Description: "test", InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (p panicTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	*p.ran = true
	panic("boom")
}

func TestPanicInAToolIsThatToolsError(t *testing.T) {
	ran := false
	registerTool(panicTool{&ran})
	s, _ := runScript(t, call("PanicProbe", `{}`), say("segui"))
	if !ran || s.State != "idle" || !strings.Contains(transcript(s), "failed inside infuse") || !strings.Contains(transcript(s), "segui") {
		t.Fatalf("ran %v, state %s: %s", ran, s.State, s.lastError)
	}
}

func TestLongWorkWithProgressIsNotCutAtAStepCount(t *testing.T) {
	n := 0
	plan := func(ModelRequest) (ModelResult, error) {
		n++
		if n > 260 {
			return ModelResult{Content: []Block{{Type: "text", Text: "fim"}}, StopReason: "end_turn"}, nil
		}
		status := "done" // the plan advances on every step
		if n%2 == 0 {
			status = "in_progress"
		}
		in := fmt.Sprintf(`{"tasks":[{"id":"1","text":"passo","status":"done"},{"id":"%d","text":"seguinte","status":"%s","proof":"teste"}]}`, n, status)
		if n == 260 {
			in = `{"tasks":[{"id":"1","text":"passo","status":"done"}]}`
		}
		return ModelResult{Content: []Block{{Type: "tool_use", ID: fmt.Sprintf("t%d", n), Name: "TodoWrite", Input: json.RawMessage(in)}}, StopReason: "tool_use"}, nil
	}
	s, _ := runScript(t, plan)
	if s.State != "idle" || n <= 200 {
		t.Fatalf("stopped after %d steps: %s %s", n, s.State, s.lastError)
	}
}

func TestNoProgressIsWarnedThenStopped(t *testing.T) {
	s, p := runScript(t, call("Glob", `{"pattern":"*.nada"}`))
	if s.State != "error" || !strings.Contains(s.lastError, "120") || !strings.Contains(transcript(s), M("stall.warn", "steps", 60).String()) {
		t.Fatalf("state %s after %d calls: %s", s.State, p.n, s.lastError)
	}
}

func TestInterruptedTurnResumesAtStartup(t *testing.T) {
	testHome(t)
	scriptSeq++
	name := fmt.Sprintf("script-%d", scriptSeq)
	p := &scriptProvider{steps: []func(ModelRequest) (ModelResult, error){say("retomei")}}
	registerProvider(name, func(ProviderConfig) Provider { return p })
	root := t.TempDir()
	RememberProject(root)
	s := newSession(root)
	s.Provider, s.Model = name, "script"
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "tarefa"}}}, {Role: "assistant", Content: []Block{{Type: "tool_use", ID: "x", Name: "Glob", Input: json.RawMessage(`{"pattern":"*"}`)}}}}
	s.Working = true
	s.save()
	sessionsMu.Lock()
	delete(sessions, s.ID) // as after a restart: only the file remains
	sessionsMu.Unlock()
	resumeInterruptedSessions()
	r, err := loadSession(root, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	waitTurn(t, r)
	if !strings.Contains(transcript(r), "retomei") || !strings.Contains(transcript(r), "infuse was restarted") {
		t.Fatalf("not resumed: %s", transcript(r))
	}
}
