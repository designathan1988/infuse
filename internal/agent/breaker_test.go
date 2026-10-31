package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// stubbornProvider repeats the same failing call; with listen, it changes approach once infuse tells it to.
type stubbornProvider struct{ listen bool }

func (stubbornProvider) Name() string { return "stubborn" }
func (p stubbornProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	last := req.Messages[len(req.Messages)-1]
	for _, b := range last.Content {
		if p.listen && b.Type == "text" && b.Text == M("breaker.first", "why", M("breaker.same", "tool", "Read")).String() {
			return ModelResult{Content: []Block{{Type: "text", Text: "Mudei a abordagem."}}, StopReason: "end_turn"}, nil
		}
	}
	in, _ := json.Marshal(map[string]string{"path": "nao-existe.txt"})
	return ModelResult{Content: []Block{{Type: "tool_use", ID: "r" + time.Now().Format("150405.000000000"), Name: "Read", Input: in}}, StopReason: "tool_use"}, nil
}

func runStubborn(t *testing.T, listen bool) *Session {
	t.Helper()
	testHome(t)
	registerProvider("stubborn-test", func(ProviderConfig) Provider { return stubbornProvider{listen} })
	s := newSession(t.TempDir())
	s.Provider, s.Model, s.Mode = "stubborn-test", "stubborn", "bypass"
	off := false
	s.settings.RequireReview = &off
	if err := s.Send("leia o arquivo", nil, nil); err != nil {
		t.Fatal(err)
	}
	started := false
	for i := 0; i < 400; i++ {
		s.Mu.Lock()
		st, n := s.State, len(s.Messages)
		s.Mu.Unlock()
		started = started || st != "idle" || n > 1
		if started && (st == "idle" || st == "error") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	return s
}

func TestBreakerSendsTheAgentBackOnce(t *testing.T) {
	s := runStubborn(t, true)
	if s.State != "idle" {
		t.Fatalf("a warned agent that changed approach ended in %s: %s", s.State, s.lastError)
	}
	b, _ := json.Marshal(s.Messages)
	if !strings.Contains(string(b), "change the approach") || !strings.Contains(string(b), "Mudei a abordagem") {
		t.Fatalf("no warning in the conversation: %s", b)
	}
	if got := repairTranscript(s.Messages); len(got) != len(s.Messages) {
		t.Fatal("the warning broke the transcript")
	}
}

func TestBreakerEndsTheTurnWhenTheAgentInsists(t *testing.T) {
	s := runStubborn(t, false)
	if s.State != "error" || s.lastError != M("breaker.same", "tool", "Read").Render(s.uiLang()) {
		t.Fatalf("state %s: %s", s.State, s.lastError)
	}
}
