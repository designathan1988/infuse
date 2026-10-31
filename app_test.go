package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	prev := secretBackend
	secretBackend = memSecrets{}
	t.Cleanup(func() { secretBackend = prev })
}

func TestConnectProviderStoresKeyInVaultNotSettings(t *testing.T) {
	testHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("authorization") != "Bearer sk-test-1234" {
			w.WriteHeader(401)
			_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"deepseek-chat"},{"id":"deepseek-reasoner"}]}`)
	}))
	defer srv.Close()
	if _, err := ConnectProvider(context.Background(), "deepseek", "wrong", srv.URL); err == nil {
		t.Fatal("bad key accepted")
	}
	c, err := ConnectProvider(context.Background(), "deepseek", "sk-test-1234", srv.URL)
	if err != nil || !c.Connected || len(c.Models) != 2 || c.KeyHint != "…1234" {
		t.Fatalf("%+v %v", c, err)
	}
	raw, _ := os.ReadFile(settingsFile())
	if strings.Contains(string(raw), "sk-test-1234") {
		t.Fatal("key written to settings.json")
	}
	if k, _ := secretGet(secretName("deepseek")); k != "sk-test-1234" {
		t.Fatal("key not in the vault")
	}
	s := LoadSettings()
	if s.Provider != "deepseek" || s.Model != "deepseek-chat" {
		t.Fatalf("default not set: %s %s", s.Provider, s.Model)
	}
	kind, cfg := connectionConfig(s, "deepseek")
	if kind != "openai" || cfg.APIKey != "sk-test-1234" || cfg.BaseURL != srv.URL {
		t.Fatalf("%s %+v", kind, cfg)
	}
	if ms := AvailableModels(s); len(ms) != 2 || ms[0].Provider != "deepseek" {
		t.Fatalf("%+v", ms)
	}
	_ = DisconnectProvider("deepseek")
	if k, _ := secretGet(secretName("deepseek")); k != "" {
		t.Fatal("key left in the vault")
	}
}

func TestLegacyKeyMigrates(t *testing.T) {
	testHome(t)
	_ = writeJSON(settingsFile(), map[string]any{"provider": "openai", "apiKey": "sk-legacy-9999", "baseUrl": "https://api.deepseek.com", "model": "deepseek-chat"})
	s := LoadSettings()
	if s.APIKey != "" || s.Provider != "deepseek" || !s.Connections["deepseek"].Connected {
		t.Fatalf("%+v", s)
	}
	raw, _ := os.ReadFile(settingsFile())
	if strings.Contains(string(raw), "sk-legacy-9999") {
		t.Fatal("legacy key still in settings.json")
	}
	if k, _ := secretGet(secretName("deepseek")); k != "sk-legacy-9999" {
		t.Fatal("legacy key lost")
	}
}

func TestRewindVariants(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	file := filepath.Join(root, "a.txt")
	_ = os.WriteFile(file, []byte("v0"), 0o644)
	s := newSession(root)
	turn := func(text, content string) {
		s.mu.Lock()
		s.Turn++
		s.TurnStarts = append(s.TurnStarts, len(s.Messages))
		s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: text}}}, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "ok"}}})
		s.mu.Unlock()
		s.checkpointBeforeWrite(file)
		_ = os.WriteFile(file, []byte(content), 0o644)
	}
	turn("primeiro", "v1")
	turn("segundo", "v2")

	// code only: file back to before turn 2, conversation intact
	p, err := s.RewindTo(2, true, false)
	if err != nil || p != "segundo" {
		t.Fatal(p, err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v1" || s.Turn != 2 || len(s.Messages) != 4 {
		t.Fatalf("code-only: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
	// conversation only: messages cut, file kept
	_ = os.WriteFile(file, []byte("v2b"), 0o644)
	if _, err := s.RewindTo(2, false, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v2b" || s.Turn != 1 || len(s.Messages) != 2 {
		t.Fatalf("conversation-only: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
	// both
	if _, err := s.RewindTo(1, true, true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); string(b) != "v0" || s.Turn != 0 || len(s.Messages) != 0 {
		t.Fatalf("both: %q turn=%d msgs=%d", b, s.Turn, len(s.Messages))
	}
}

func TestSchedulerNextRun(t *testing.T) {
	loc := time.Local
	mon := time.Date(2026, 11, 2, 10, 0, 0, 0, loc) // a Monday
	daily := ScheduledTask{Active: true, Preset: "daily", Hour: 9, Minute: 30}
	if got := time.UnixMilli(nextRun(daily, mon)); got.Day() != 3 || got.Hour() != 9 || got.Minute() != 30 {
		t.Fatalf("daily %v", got)
	}
	fri := time.Date(2026, 11, 6, 18, 0, 0, 0, loc)
	wk := ScheduledTask{Active: true, Preset: "weekdays", Hour: 8}
	if got := time.UnixMilli(nextRun(wk, fri)); got.Weekday() != time.Monday {
		t.Fatalf("weekdays %v", got)
	}
	hourly := ScheduledTask{Active: true, Preset: "hourly", Minute: 5}
	if got := time.UnixMilli(nextRun(hourly, mon)); got.Hour() != 10 || got.Minute() != 5 {
		t.Fatalf("hourly %v", got)
	}
	weekly := ScheduledTask{Active: true, Preset: "weekly", Weekday: 3, Hour: 7}
	if got := time.UnixMilli(nextRun(weekly, mon)); got.Weekday() != time.Wednesday {
		t.Fatalf("weekly %v", got)
	}
	if nextRun(ScheduledTask{Active: false, Preset: "daily"}, mon) != 0 || nextRun(ScheduledTask{Active: true, Preset: "manual"}, mon) != 0 {
		t.Fatal("paused/manual must not schedule")
	}
}

func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}} {
		if out, err := gitRun(root, args...); err != nil {
			t.Fatal(out)
		}
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("one\ntwo\n"), 0o644)
	_, _ = gitRun(root, "add", "-A")
	if out, err := gitRun(root, "commit", "-qm", "init"); err != nil {
		t.Fatal(out)
	}
	return root
}

func TestWorktreeAndChanges(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	dir, branch, err := CreateWorktree(root, "Minha Feature!")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(branch, "infuse/minha-feature") || !exists(filepath.Join(dir, "a.txt")) {
		t.Fatalf("%s %s", dir, branch)
	}
	if st := gitOutput(root, "status", "--porcelain"); strings.TrimSpace(st) != "" {
		t.Fatalf("worktree folder shows as a change in the project: %q", st)
	}
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\nTWO\nthree\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("x\ny\n"), 0o644)
	c := Changes(dir)
	files := c["files"].([]fileChange)
	if len(files) != 2 || c["added"].(int) != 4 || c["removed"].(int) != 1 {
		t.Fatalf("%+v", c)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "a.txt")); string(b) != "one\ntwo\n" {
		t.Fatal("worktree edit leaked into the main checkout")
	}
	// the oracle inside a worktree is protected too
	g := newGuard(dir, nil)
	if _, ok := g.protects(dir, ".infuse/oracle/pronto.md"); !ok {
		t.Fatal("worktree oracle not protected")
	}
	if _, ok := g.protects(dir, filepath.Join(root, ".infuse", "oracle", "x.md")); !ok {
		t.Fatal("main oracle reachable from a worktree")
	}
	if err := RemoveWorktree(root, dir, true); err != nil {
		t.Fatal(err)
	}
}

func TestUserFileWriteConflict(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	m, err := WriteUserFile(root, "n.txt", "a", 0, false)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_ = os.WriteFile(filepath.Join(root, "n.txt"), []byte("disk"), 0o644)
	if _, err := WriteUserFile(root, "n.txt", "mine", m-1000, false); err != errConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err := WriteUserFile(root, "n.txt", "mine", m-1000, true); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteUserFile(root, "../x.txt", "no", 0, true); err == nil {
		t.Fatal("write outside the project accepted")
	}
}

func TestTerminalEcho(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("no console in CI")
	}
	dir := t.TempDir()
	term, err := OpenTerminal(dir, dir, 80, 24)
	if err != nil {
		t.Skip("pty unavailable: " + err.Error())
	}
	defer CloseTerminal(term.ID)
	_, _ = term.pty.Write([]byte("echo infuse-term-$((40+2))\r"))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		term.mu.Lock()
		out := string(term.backlog)
		term.mu.Unlock()
		if strings.Contains(out, "infuse-term-42") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	term.mu.Lock()
	defer term.mu.Unlock()
	t.Fatalf("no echo from the shell; got %q", string(term.backlog))
}
