package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Built-in slash commands are a data table with a group and locus; custom commands join the same list.
func TestBuiltinCommandsComeFromTheTable(t *testing.T) {
	root := t.TempDir()
	cmds := listCommands(root)
	if len(cmds) < 20 {
		t.Fatalf("the builtin table is too small: %d", len(cmds))
	}
	byName := map[string]slashCommand{}
	for _, c := range cmds {
		byName[c.Name] = c
	}
	if m := byName["model"]; !m.Builtin || m.Group != "model" || m.Locus != "client" {
		t.Fatalf("model: %+v", m)
	}
	if h := byName["help"]; h.Group != "help" {
		t.Fatalf("help: %+v", h)
	}
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\ndescription: Deploy the app\n---\nDeploy with $ARGUMENTS\n"
	if err := os.WriteFile(filepath.Join(root, ".infuse", "commands", "deploy.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmds = listCommands(root)
	for _, c := range cmds {
		if c.Name != "deploy" {
			continue
		}
		if c.Builtin || c.Group != "custom" || c.Locus != "custom" || !strings.Contains(c.Prompt, "$ARGUMENTS") {
			t.Fatalf("custom command: %+v", c)
		}
		return
	}
	t.Fatal("the custom command did not join the registry")
}
