package agent

import (
	"encoding/json"
	"infuse/internal/config"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var convo = []Message{
	{Role: "user", Content: []Block{{Type: "text", Text: "oi"}}},
	{Role: "assistant", Content: []Block{{Type: "text", Text: "vou ler"}, {Type: "tool_use", ID: "c1", Name: "Read", Input: json.RawMessage(`{"path":"a"}`)}}},
	{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "c1", Content: "conteúdo"}}},
}

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	prev := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	t.Cleanup(func() { config.SecretBackend = prev })
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

func writeSpec(t *testing.T, root, spec string) {
	t.Helper()
	p := filepath.Join(root, ".infuse", "oracle", "checks.json")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
}
