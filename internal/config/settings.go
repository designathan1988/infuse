package config

import (
	"infuse/internal/core"
	"path/filepath"
	"strings"
	"sync"
)

// Settings is the canonical configuration. Global file: ~/.infuse/settings.json.
// A project may override a subset in <project>/.infuse/settings.json.
type Settings struct {
	Provider             string                `json:"provider"` // default connection id
	Connections          map[string]Connection `json:"connections"`
	Effort               string                `json:"effort"`            // default effort: low | medium | high | max
	WorktreeDir          string                `json:"worktreeDir"`       // where session worktrees go, relative to the project
	BranchPrefix         string                `json:"branchPrefix"`      // prefix of worktree branches
	APIKey               string                `json:"apiKey,omitempty"`  // legacy (0.5): migrated to the OS secret store
	BaseURL              string                `json:"baseUrl,omitempty"` // legacy (0.5)
	Model                string                `json:"model"`
	FastModel            string                `json:"fastModel"`
	MaxTokens            int                   `json:"maxTokens"`
	ContextLimit         int                   `json:"contextLimit"`
	ThinkingBudget       int                   `json:"thinkingBudget"`
	Mode                 string                `json:"mode"`        // default | acceptEdits | plan | bypass
	Permissions          map[string]string     `json:"permissions"` // tool → ask | allow | deny
	ProtectedPaths       []string              `json:"protectedPaths"`
	Pricing              map[string][2]float64 `json:"pricing"` // model prefix → [input, output] USD per MTok
	DailyLimitUSD        float64               `json:"dailyLimitUsd"`
	Language             string                `json:"language"`
	HideTray             bool                  `json:"hideTray"`                       // no tray icon
	CloseToTray          bool                  `json:"closeToTray"`                    // closing the window keeps the app in the tray
	Notify               bool                  `json:"notify"`                         // OS notification when the agent needs you or finishes
	Vim                  bool                  `json:"vim"`                            // vim keys in the prompt box
	Theme                string                `json:"theme"`                          // system | light | dark
	ShowThinking         *bool                 `json:"showThinking,omitempty"`         // reasoning blocks open by default (nil = yes)
	PromptSuggest        *bool                 `json:"promptSuggestions,omitempty"`    // gray next-message suggestion in the prompt box (nil = yes)
	Diagnostics          *bool                 `json:"diagnostics,omitempty"`          // check each written file with the project's checker (nil = yes)
	EnforceConventions   *bool                 `json:"enforceConventions,omitempty"`   // refuse writes that break the project's conventions (nil = yes)
	RequireReview        *bool                 `json:"requireReview,omitempty"`        // independent review at the end of each turn with changes (nil = yes)
	MaxFileLines         int                   `json:"maxFileLines,omitempty"`         // refuse code files longer than this (0 = 400, -1 = no limit)
	UserProhibitions     *bool                 `json:"userProhibitions,omitempty"`     // the user's prohibitions compiled and enforced (nil = on)
	RequireDiagnosis     *bool                 `json:"requireDiagnosis,omitempty"`     // a verified diagnosis before changes and commands (nil = on)
	ReadProjectKnowledge *bool                 `json:"readProjectKnowledge,omitempty"` // infuse reads the project documents to the end (nil = on)
	EnforceArchitecture  *bool                 `json:"enforceArchitecture,omitempty"`  // architecture rules on every write (nil = on)
	BrowserViewport      string                `json:"browserViewport,omitempty"`      // agent browser window, e.g. 1440x900
	BrowserScale         float64               `json:"browserScale,omitempty"`         // pixel density of its captures (0 = 2)
	BrowserGestures      string                `json:"browserGestures,omitempty"`      // human (default) | slow | instant
}

func DefaultSettings() Settings {
	return Settings{
		Provider:       "",
		Connections:    map[string]Connection{},
		Effort:         "medium",
		WorktreeDir:    ".infuse/worktrees",
		BranchPrefix:   "infuse/",
		Model:          "claude-sonnet-5-5",
		FastModel:      "claude-haiku-5-5",
		MaxTokens:      16000,
		ContextLimit:   0, // the model's own window
		Mode:           "default",
		Permissions:    map[string]string{"Bash": "ask", "Write": "ask", "Edit": "ask", "WebFetch": "ask"},
		ProtectedPaths: []string{".infuse/oracle/**"},
		Pricing: map[string][2]float64{
			"claude-opus":      {15, 75},
			"claude-sonnet":    {3, 15},
			"claude-haiku":     {1, 5},
			"claude-fable":     {15, 75},
			"gpt-5":            {1.25, 10},
			"gpt-4.1":          {2, 8},
			"gpt-4o":           {2.5, 10},
			"o3":               {2, 8},
			"gemini-2.5-pro":   {1.25, 10},
			"gemini-2.5-flash": {0.3, 2.5},
			"deepseek-flash":    {0.3, 1.2},
			"deepseek-v4-flash": {0.3, 1.2},
			"deepseek-chat":     {0.3, 1.2},
			"deepseek-reasoner": {0.3, 1.2},
			"deepseek-v4-pro":   {1.32, 3.96},
		},
		DailyLimitUSD: 20,
		Language:      "pt-BR",
		Notify:        true,
		Theme:         "system",
	}
}

var settingsMu sync.Mutex

func SettingsFile() string { return filepath.Join(core.InfuseHome(), "settings.json") }

func LoadSettings() Settings {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s := DefaultSettings()
	_ = core.ReadJSON(SettingsFile(), &s)
	if s.Permissions == nil {
		s.Permissions = DefaultSettings().Permissions
	}
	if s.Pricing == nil {
		s.Pricing = DefaultSettings().Pricing
	}
	for k, v := range DefaultSettings().Pricing { // prices added in later versions reach older settings files
		if _, ok := s.Pricing[k]; !ok {
			s.Pricing[k] = v
		}
	}
	delete(s.Pricing, "claude-mythos")
	if s.MaxTokens <= 0 {
		s.MaxTokens = 16000
	}
	if s.ContextLimit < 0 || s.ContextLimit == 200000 {
		s.ContextLimit = 0
	}
	if s.Model == "" {
		s.Model = DefaultSettings().Model
	}
	if s.Connections == nil {
		s.Connections = map[string]Connection{}
	}
	if s.WorktreeDir == "" {
		s.WorktreeDir = ".infuse/worktrees"
	}
	if s.BranchPrefix == "" {
		s.BranchPrefix = "infuse/"
	}
	if s.Provider == "mock" && s.Connections["mock"].Kind == "" {
		s.Connections["mock"] = Connection{Kind: "mock", Connected: true, Models: []string{"mock"}}
	}
	if MigrateLegacyKey(&s) {
		_ = core.WriteJSON(SettingsFile(), s)
	}
	return s
}

func SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	return core.WriteJSON(SettingsFile(), s)
}

// ProjectSettings: the subset a project may override.
type ProjectSettings struct {
	Model          string            `json:"model,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Permissions    map[string]string `json:"permissions,omitempty"`
	ProtectedPaths []string          `json:"protectedPaths,omitempty"`
}

func EffectiveSettings(root string) Settings {
	s := LoadSettings()
	var p ProjectSettings
	if core.ReadJSON(filepath.Join(root, core.InfuseDirName, "settings.json"), &p) == nil {
		if p.Model != "" {
			s.Model = p.Model
		}
		if p.Mode != "" {
			s.Mode = p.Mode
		}
		for k, v := range p.Permissions {
			s.Permissions[k] = v
		}
		s.ProtectedPaths = append(s.ProtectedPaths, p.ProtectedPaths...)
	}
	return s
}

// RootLanguage is the language the user reads infuse in, for code that has a project folder but no
// session.
func RootLanguage(root string) string {
	if l := EffectiveSettings(root).Language; l != "" {
		return l
	}
	return "pt-BR"
}

func (s Settings) priceFor(model string) (float64, float64) {
	best := ""
	for prefix := range s.Pricing {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return 0, 0
	}
	return s.Pricing[best][0], s.Pricing[best][1]
}

// HasPrice reports whether the model's price is known (an unknown price counts as zero cost).
func (s Settings) HasPrice(model string) bool { in, out := s.priceFor(model); return in > 0 || out > 0 }

func (s Settings) CostOf(model string, u core.Usage) float64 {
	in, out := s.priceFor(model)
	return (float64(u.Input)+float64(u.CacheWrite)*1.25+float64(u.CacheRead)*0.1)*in/1e6 + float64(u.Output)*out/1e6
}
