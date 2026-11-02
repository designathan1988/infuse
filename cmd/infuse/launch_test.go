package main

import (
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func env(t *testing.T, list []string, key string) string {
	t.Helper()
	return envValue(list, key)
}

// Each agent gets the variables its own source reads, and no pretend ones.
func TestAgentEnvSetsWhatEachCLIReads(t *testing.T) {
	base := []string{"Path=C:\\Windows", "ComSpec=C:\\Windows\\system32\\cmd.exe", "SHELL=/bin/zsh"}
	shim, ps := filepath.Join("h", "bin", "bash.exe"), filepath.Join("h", "bin", "ps", "pwsh.exe")

	claude := agentEnv("claude", base, shim, ps, "windows")
	if env(t, claude, "CLAUDE_CODE_GIT_BASH_PATH") != shim || env(t, claude, "SHELL") != shim {
		t.Fatalf("%v", claude)
	}
	codex := agentEnv("codex", base, shim, ps, "windows")
	path := env(t, codex, "PATH")
	if !strings.HasPrefix(path, filepath.Dir(ps)+string(os.PathListSeparator)+filepath.Dir(shim)) || !strings.HasSuffix(path, "C:\\Windows") {
		t.Fatalf("the PowerShell shim is not first on PATH: %s", path)
	}
	if env(t, codex, "SHELL") != "/bin/zsh" || env(t, codex, "CLAUDE_CODE_SHELL") != "" {
		t.Fatalf("variables Codex never reads were set: %v", codex)
	}
	gemini := agentEnv("gemini", base, shim, ps, "windows")
	if !strings.HasSuffix(env(t, gemini, "ComSpec"), "cmd.exe") {
		t.Fatalf("a ComSpec of cmd.exe is left alone: %v", gemini)
	}
	ps1 := agentEnv("gemini", []string{"PATH=x", "ComSpec=C:\\pwsh\\pwsh.exe"}, shim, ps, "windows")
	if env(t, ps1, "ComSpec") != ps {
		t.Fatalf("Gemini prefers ComSpec when it names PowerShell: %v", ps1)
	}
	unix := agentEnv("gemini", []string{"PATH=/usr/bin"}, shim, ps, "linux")
	if !strings.HasPrefix(env(t, unix, "PATH"), filepath.Dir(shim)+string(os.PathListSeparator)) {
		t.Fatalf("bash shim not on PATH: %v", unix)
	}
}

// Mounting refreshes the map, and writes the grounds in the file that agent reads, keeping what the
// user wrote there.
func TestMountEnvironmentWritesTheGroundsBlock(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	t.Setenv("INFUSE_WATCHER_IDLE_MIN", "1")
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".infuse"), 0o755)
	// a live pid in the state file: the test binary must not start a watcher of itself
	_ = os.WriteFile(filepath.Join(root, ".infuse", "watcher.json"), []byte(`{"pid":`+strconv.Itoa(os.Getpid())+`}`), 0o644)
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("export const a = 1;\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Mine\n"), 0o644)
	for kind, file := range groundsFile {
		if err := mountEnvironment(kind, root); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "infuse:begin grounds") || !strings.Contains(string(b), "a.js") {
			t.Fatalf("%s: %s", file, b)
		}
	}
	got, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !strings.HasPrefix(string(got), "# Mine\n") {
		t.Fatalf("%s", got)
	}
	if projectmap.LoadMap(root) == nil {
		t.Fatal("the map was not refreshed")
	}
}
