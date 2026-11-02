package config

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managedAt(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "managed-settings.json")
	old := ManagedSettingsPath
	ManagedSettingsPath = func() string { return p }
	t.Cleanup(func() { ManagedSettingsPath = old })
	return p
}

func writeLayer(t *testing.T, file string, v map[string]any) {
	t.Helper()
	if err := core.WriteJSON(file, v); err != nil {
		t.Fatal(err)
	}
}

// The local file lives in the project: like the project's, it only tightens.
func TestLocalLayerCannotLoosenThePolicy(t *testing.T) {
	testHome(t)
	managedAt(t)
	root := t.TempDir()
	writeLayer(t, SettingsFile(), map[string]any{"mode": "default", "permissions": map[string]string{"Bash": "ask"}})
	writeLayer(t, ProjectSettingsFile(root), map[string]any{"permissions": map[string]string{"Write": "deny"}})
	writeLayer(t, LocalSettingsFile(root), map[string]any{
		"mode": "bypass", "model": "local-model",
		"permissions": map[string]string{"Bash": "allow", "Write": "allow", "Edit": "deny", "Bash(git status:*)": "allow"},
	})
	s := EffectiveSettings(root)
	if s.Mode != "default" || s.Permissions["Bash"] != "ask" || s.Permissions["Write"] != "deny" {
		t.Fatalf("a local file loosened the policy: %s %+v", s.Mode, s.Permissions)
	}
	if s.Permissions["Bash(git status:*)"] == "allow" {
		t.Fatal("a pattern rule in the project allowed a command")
	}
	if s.Permissions["Edit"] != "deny" || s.Model != "local-model" {
		t.Fatalf("%+v %s", s.Permissions, s.Model)
	}
}

func TestManagedLayerWinsOverEveryLayer(t *testing.T) {
	testHome(t)
	managed := managedAt(t)
	root := t.TempDir()
	writeLayer(t, SettingsFile(), map[string]any{"mode": "bypass", "permissions": map[string]string{"Bash": "allow"}, "env": map[string]string{"A": "user"}})
	writeLayer(t, managed, map[string]any{
		"mode": "acceptEdits", "permissions": map[string]string{"Bash": "deny", "Bash(ls:*)": "allow"},
		"protectedPaths": []string{"secret/**"}, "env": map[string]string{"A": "managed", "PATH": "/opt/bin"},
	})
	s := EffectiveSettings(root)
	if s.Mode != "acceptEdits" || s.Permissions["Bash"] != "deny" || s.Permissions["Bash(ls:*)"] != "allow" {
		t.Fatalf("%s %+v", s.Mode, s.Permissions)
	}
	if s.Env["A"] != "managed" || s.Env["PATH"] != "/opt/bin" {
		t.Fatalf("%+v", s.Env)
	}
	if !strings.Contains(strings.Join(s.ProtectedPaths, " "), "secret/**") {
		t.Fatal(s.ProtectedPaths)
	}
	if s.CapMode("bypass") != "acceptEdits" || s.CapMode("plan") != "plan" {
		t.Fatal("the managed mode is a ceiling:", s.CapMode("bypass"))
	}
}

func TestProjectEnvMergesAndCannotHijackTools(t *testing.T) {
	testHome(t)
	managedAt(t)
	root := t.TempDir()
	writeLayer(t, SettingsFile(), map[string]any{"env": map[string]string{"SHARED": "user", "TOOL_URL": "u"}})
	writeLayer(t, ProjectSettingsFile(root), map[string]any{"env": map[string]string{
		"SHARED": "project", "API_BASE": "https://x", "PATH": "/evil", "LD_PRELOAD": "/evil.so", "BASH_ENV": "/evil.sh", "GIT_SSH_COMMAND": "evil", "INFUSE_TOKEN": "x",
	}})
	writeLayer(t, LocalSettingsFile(root), map[string]any{"env": map[string]string{"SHARED": "local"}})
	s := EffectiveSettings(root)
	if s.Env["SHARED"] != "local" || s.Env["API_BASE"] != "https://x" || s.Env["TOOL_URL"] != "u" {
		t.Fatalf("%+v", s.Env)
	}
	for _, k := range []string{"PATH", "LD_PRELOAD", "BASH_ENV", "GIT_SSH_COMMAND", "INFUSE_TOKEN"} {
		if _, ok := s.Env[k]; ok {
			t.Fatalf("a project set %s", k)
		}
	}
}

func TestLocalSettingsStayUnversioned(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := SaveLocalSettings(root, ProjectSettings{Env: map[string]string{"K": "v"}}); err != nil {
		t.Fatal(err)
	}
	if err := SaveLocalSettings(root, ProjectSettings{Env: map[string]string{"K": "w"}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(root, core.InfuseDirName, ".gitignore"))
	if err != nil || strings.Count(string(b), LocalSettingsName) != 1 {
		t.Fatalf("%q %v", b, err)
	}
	layers := SettingsLayers(root)
	if len(layers) != 4 || layers[2].Name != "local" || !layers[2].Exists || layers[2].Settings.Env["K"] != "w" {
		t.Fatalf("%+v", layers)
	}
}
