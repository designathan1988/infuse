package config

// Layered settings, from the weakest: the defaults, the user's file, the project's file, the local file
// (.infuse/settings.local.json, personal and never versioned), the machine's managed file. The project
// and local files are data inside the project: each may tighten the user's policy, never loosen it.
// The managed file is the machine owner's: it is outside the project and overrides everything.

import (
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// LocalSettingsName is the local layer's file, inside .infuse.
const LocalSettingsName = "settings.local.json"

// ManagedSettingsPath names the machine-wide file; a variable so tests can point it elsewhere.
var ManagedSettingsPath = defaultManagedPath

func defaultManagedPath() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("ProgramData"), "infuse", "managed-settings.json")
	case "darwin":
		return "/Library/Application Support/infuse/managed-settings.json"
	}
	return "/etc/infuse/managed-settings.json"
}

// ProjectSettingsFile and LocalSettingsFile are the two layers that live in the project.
func ProjectSettingsFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "settings.json")
}

func LocalSettingsFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, LocalSettingsName)
}

// reservedEnv are variables a file in the project may not set: each one runs code or redirects tools
// before any approval (a cloned repository would otherwise choose what every command executes).
var reservedEnv = []string{"PATH", "PATHEXT", "BASH_ENV", "ENV", "SHELL", "HOME", "USERPROFILE", "COMSPEC", "PROMPT_COMMAND", "NODE_OPTIONS", "PYTHONPATH", "PYTHONSTARTUP", "RUBYOPT", "PERL5OPT"}

var reservedEnvPrefixes = []string{"LD_", "DYLD_", "GIT_", "INFUSE_"}

// ReservedEnv reports whether a project may not set the variable.
func ReservedEnv(name string) bool {
	up := strings.ToUpper(name)
	for _, r := range reservedEnv {
		if up == r {
			return true
		}
	}
	for _, p := range reservedEnvPrefixes {
		if strings.HasPrefix(up, p) {
			return true
		}
	}
	return false
}

// applyProjectLayer merges one project-side file over the settings, stricter wins.
func applyProjectLayer(s *Settings, p ProjectSettings) {
	if p.Model != "" {
		s.Model = p.Model
	}
	if p.OutputStyle != "" {
		s.OutputStyle = p.OutputStyle
	}
	// A project file is data inside the project: it may make the agent more restricted, never
	// less. Otherwise one write to .infuse/settings.json would turn the filter off.
	if p.Mode != "" && modeRank(p.Mode) <= modeRank(s.Mode) {
		s.Mode = p.Mode
	}
	for k, v := range p.Permissions {
		current, ok := s.Permissions[k]
		if !ok {
			current = "ask"
		}
		if permRank(v) <= permRank(current) {
			s.Permissions[k] = v
		}
	}
	s.ProtectedPaths = append(s.ProtectedPaths, p.ProtectedPaths...) // additions only tighten
	for k, v := range p.Env {
		if !ReservedEnv(k) {
			setEnv(s, k, v)
		}
	}
}

// applyManaged puts the machine's file over everything, and its mode becomes a ceiling for the session.
func applyManaged(s *Settings, p ProjectSettings) {
	if p.Model != "" {
		s.Model = p.Model
	}
	if p.OutputStyle != "" {
		s.OutputStyle = p.OutputStyle
	}
	if p.Mode != "" {
		s.Mode, s.ModeCeiling = p.Mode, p.Mode
	}
	for k, v := range p.Permissions {
		s.Permissions[k] = v
	}
	s.ProtectedPaths = append(s.ProtectedPaths, p.ProtectedPaths...)
	for k, v := range p.Env {
		setEnv(s, k, v)
	}
}

func setEnv(s *Settings, k, v string) {
	if s.Env == nil {
		s.Env = map[string]string{}
	}
	s.Env[k] = v
}

func readLayer(file string, p *ProjectSettings) bool {
	if core.ReadJSON(file, p) != nil {
		return false
	}
	_, _ = p.Upgrade(nil)
	return true
}

// CapMode lowers a mode to the machine's ceiling, when it has one.
func (s Settings) CapMode(mode string) string {
	if s.ModeCeiling != "" && modeRank(mode) > modeRank(s.ModeCeiling) {
		return s.ModeCeiling
	}
	return mode
}

// SettingsLayer describes one layer for the settings screen.
type SettingsLayer struct {
	Name     string          `json:"name"` // managed | user | project | local
	File     string          `json:"file"`
	Exists   bool            `json:"exists"`
	Settings ProjectSettings `json:"settings"`
}

// SettingsLayers lists the layers from the weakest to the strongest.
func SettingsLayers(root string) []SettingsLayer {
	layers := []SettingsLayer{
		{Name: "user", File: SettingsFile()},
		{Name: "project", File: ProjectSettingsFile(root)},
		{Name: "local", File: LocalSettingsFile(root)},
		{Name: "managed", File: ManagedSettingsPath()},
	}
	for i := range layers {
		layers[i].Exists = readLayer(layers[i].File, &layers[i].Settings)
	}
	return layers
}

// SaveLocalSettings writes the local layer, and keeps it out of version control.
func SaveLocalSettings(root string, p ProjectSettings) error {
	p.SchemaVersion = store.Version
	if err := core.WriteJSON(LocalSettingsFile(root), p); err != nil {
		return err
	}
	return ignoreLocalSettings(root)
}

// ignoreLocalSettings adds the file to .infuse/.gitignore: a project that versions .infuse must not
// version one person's overrides or their variables.
func ignoreLocalSettings(root string) error {
	p := filepath.Join(root, core.InfuseDirName, ".gitignore")
	b, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == LocalSettingsName {
			return nil
		}
	}
	text := string(b)
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return os.WriteFile(p, []byte(text+LocalSettingsName+"\n"), 0o644)
}
