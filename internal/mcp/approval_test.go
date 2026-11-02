package mcp

import (
	"infuse/internal/config"
	"os"
	"path/filepath"
	"testing"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	old := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	t.Cleanup(func() { config.SecretBackend = old })
}

func script(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The approval covers what runs: an approved "node tools/mcp.js" whose file changed needs a new one.
func TestEditedScriptLosesItsApproval(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	script(t, root, "tools/mcp.js", "console.log('v1')\n")
	cfg := ServerConfig{Command: "node", Args: []string{"tools/mcp.js"}}
	if err := Approve(root, "project", "s", cfg); err != nil {
		t.Fatal(err)
	}
	if !Approved(root, "project", "s", cfg) {
		t.Fatal("approval was not recorded")
	}
	script(t, root, "tools/mcp.js", "console.log('v2')\n") // same size
	if Approved(root, "project", "s", cfg) {
		t.Fatal("an edited script ran on an old approval")
	}
	script(t, root, "tools/mcp.js", "console.log('v1')\n")
	if !Approved(root, "project", "s", cfg) {
		t.Fatal("the approved content must stay approved")
	}
}

// A flag's value and a program inside the project count the same as a plain argument.
func TestLaunchedFilesAreFoundInEveryPosition(t *testing.T) {
	root := t.TempDir()
	script(t, root, "srv/entry.js", "x")
	script(t, root, "bin/server", "y")
	for name, cfg := range map[string]ServerConfig{
		"argument": {Command: "node", Args: []string{"srv/entry.js"}},
		"flag":     {Command: "node", Args: []string{"--entry=srv/entry.js"}},
		"absolute": {Command: "node", Args: []string{filepath.Join(root, "srv", "entry.js")}},
		"program":  {Command: "bin/server"},
	} {
		if got := launchedScripts(root, cfg); len(got) != 1 {
			t.Fatalf("%s: %v", name, got)
		}
	}
}

// A file outside the project and an HTTP server have nothing to hash.
func TestNothingOutsideTheProjectIsHashed(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "mcp.js")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := launchedScripts(root, ServerConfig{Command: "node", Args: []string{outside, "../up.js", "-y"}}); len(got) != 0 {
		t.Fatalf("%v", got)
	}
	if got := launchedScripts(root, ServerConfig{Type: "http", URL: "http://x", Args: []string{"tools/mcp.js"}}); got != nil {
		t.Fatalf("%v", got)
	}
}

func TestContainmentFollowsWhoDefinesTheServer(t *testing.T) {
	root := t.TempDir()
	script(t, root, "tools/mcp.js", "x")
	if !projectDefined(root, "project", ServerConfig{Command: "npx"}) {
		t.Fatal("a project's server runs contained")
	}
	if !projectDefined(root, "global", ServerConfig{Command: "node", Args: []string{"tools/mcp.js"}}) {
		t.Fatal("a global entry launching a project file runs contained")
	}
	if projectDefined(root, "global", ServerConfig{Command: "npx", Args: []string{"-y", "some-server"}}) {
		t.Fatal("the user's own server is the user's")
	}
}
