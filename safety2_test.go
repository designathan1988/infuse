package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderRetriesRateLimit(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
			return
		}
		w.Header().Set("content-type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer srv.Close()
	p, _ := newProvider("openai", ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	res, err := p.Stream(context.Background(), ModelRequest{Model: "m", Messages: convo}, func(StreamEvent) {})
	if err != nil || len(res.Content) == 0 || res.Content[0].Text != "ok" || atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("%+v %v calls=%d", res, err, calls)
	}
}

func TestReadOutsideProjectAsks(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	in := func(p string) json.RawMessage { b, _ := json.Marshal(map[string]string{"path": p}); return b }
	if s.readsOutside(in("src/a.go")) || s.readsOutside(in(filepath.Join(root, "x"))) {
		t.Fatal("inside path reported outside")
	}
	if !s.readsOutside(in(filepath.Join(filepath.Dir(root), "other", ".ssh", "id_rsa"))) || !s.readsOutside(in(root+"-2/secret")) {
		t.Fatal("outside path not detected")
	}
}

func TestCheckpointsFollowTheWorktree(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	wt, branch, err := CreateWorktree(root, "cp")
	if err != nil {
		t.Fatal(err)
	}
	defer RemoveWorktree(root, wt, true)
	s := newSession(root)
	s.Worktree, s.Branch, s.Cwd = wt, branch, wt
	file := filepath.Join(wt, "a.txt")
	s.mu.Lock()
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "x"}}}}
	s.mu.Unlock()
	s.checkpointBeforeWrite(file)
	if p := s.Checkpoints[1][0].Path; p != "a.txt" {
		t.Fatalf("checkpoint path not relative to the worktree: %q", p)
	}
	_ = os.WriteFile(file, []byte("changed\n"), 0o644)
	if _, err := s.RewindTo(1, true, false); err != nil {
		t.Fatal(err)
	}
	lf := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") } // git may check out CRLF on Windows
	if b, _ := os.ReadFile(file); lf(b) != "one\ntwo\n" {
		t.Fatalf("worktree file not restored: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.txt")); lf(b) != "one\ntwo\n" {
		t.Fatal("main checkout touched")
	}
}

func TestSlowSubscriberIsDisconnectedNotSkipped(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	ch, _ := s.subscribe()
	for i := 0; i < 300; i++ { // nobody reads: the buffer (256) fills
		s.emit("x", i)
	}
	n := 0
	for range ch { // must end: the channel is closed, so the client reconnects for a snapshot
		n++
	}
	if n != 256 {
		t.Fatalf("got %d events before close", n)
	}
}
