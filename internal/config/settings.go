package config

import (
	"infuse/internal/core"
	"infuse/internal/store"
	"path/filepath"
	"sync"
)

// Settings is the canonical configuration. Global file: ~/.infuse/settings.json.
// A project may override a subset in <project>/.infuse/settings.json.
type Settings struct {
	store.Versioned
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
	ContainProcesses     *bool                 `json:"containProcesses,omitempty"`     // the agent's shell processes run OS-contained (nil = yes)
	RequireContract      *bool                 `json:"requireContract,omitempty"`      // the first code write of a request needs its confirmed Contract (nil = yes)
	MaxFileLines         int                   `json:"maxFileLines,omitempty"`         // refuse code files longer than this (0 = 400, -1 = no limit)
	UserProhibitions     *bool                 `json:"userProhibitions,omitempty"`     // the user's prohibitions compiled and enforced (nil = on)
	RequireDiagnosis     *bool                 `json:"requireDiagnosis,omitempty"`     // a verified diagnosis before changes and commands (nil = on)
	ReadProjectKnowledge *bool                 `json:"readProjectKnowledge,omitempty"` // infuse reads the project documents to the end (nil = on)
	EnforceArchitecture  *bool                 `json:"enforceArchitecture,omitempty"`  // architecture rules on every write (nil = on)
	BrowserViewport      string                `json:"browserViewport,omitempty"`      // agent browser window, e.g. 1440x900
	BrowserScale         float64               `json:"browserScale,omitempty"`         // pixel density of its captures (0 = 2)
	BrowserGestures      string                `json:"browserGestures,omitempty"`      // human (default) | slow | instant
	Env                  map[string]string     `json:"env,omitempty"`                  // variables for Bash and hooks, never shown to the model
	OutputStyle          string                `json:"outputStyle,omitempty"`          // the agent's output style (catalog or .infuse/output-styles)
	StatusLine           *StatusLine           `json:"statusLine,omitempty"`           // the configurable status line
	SearchProvider       string                `json:"searchProvider,omitempty"`       // WebSearch's provider ("" = the built-in chain)
	SearchProviders      []SearchProvider      `json:"searchProviders,omitempty"`      // user-defined search providers
	OTLPEndpoint         string                `json:"otlpEndpoint,omitempty"`         // OTLP/HTTP JSON collector; empty = no export
	MarketplaceURL       string                `json:"marketplaceUrl,omitempty"`       // plugin marketplace: a JSON index
	ModeCeiling          string                `json:"-"`                              // the machine's mode: a session cannot go past it
}

// StatusLine renders the status line: a command's first line of output, or a template over the
// session's fields ({model}, {mode}, {branch}, {cwd}, {cost}, {tokens}, {context}).
type StatusLine struct {
	Command  string `json:"command,omitempty"`
	Template string `json:"template,omitempty"`
}

// SearchProvider is one row of the web search table: a URL template with {query}, and how to read the
// response ("html": result links scraped from the page; "json": the fields below).
type SearchProvider struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Format   string `json:"format"`             // html | json
	Results  string `json:"results,omitempty"`  // json: the array's key path, dotted
	Title    string `json:"title,omitempty"`    // json: each result's title key
	Link     string `json:"link,omitempty"`     // json: each result's URL key
	Snippet  string `json:"snippet,omitempty"`  // json: each result's text key
	Header   string `json:"header,omitempty"`   // an authentication header's name
	KeyVault string `json:"keyVault,omitempty"` // the vault entry holding that header's value
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
			"claude-fable":      {10, 50},
			"claude-fable-5-1":  {10, 50},
			"claude-fable-5":    {10, 50},
			"claude-mythos-5-1": {10, 50},
			"claude-mythos-5":   {10, 50},
			"claude-opus":       {15, 75}, // the Opus 4.1 and 4.0 generation
			"claude-opus-5-5":   {4, 20},
			"claude-opus-5":     {5, 25},
			"claude-opus-4-8":   {5, 25},
			"claude-opus-4-7":   {5, 25},
			"claude-opus-4-6":   {5, 25},
			"claude-sonnet":     {3, 15}, // the Sonnet 4 generation
			"claude-sonnet-5-5": {2, 10},
			"claude-sonnet-5":   {2, 10},
			"claude-sonnet-4-6": {3, 15},
			"claude-haiku":      {1, 5},
			"claude-haiku-5-5":  {0.1, 0.5},
			"claude-haiku-4-5":  {1, 5},
			"gpt-5":             {1.25, 10},
			"gpt-4.1":           {2, 8},
			"gpt-4o":            {2.5, 10},
			"o3":                {2, 8},
			"gemini-2.5-pro":    {1.25, 10},
			"gemini-2.5-flash":  {0.3, 2.5},
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
	exists := core.Exists(SettingsFile())
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
	changed, _ := s.Upgrade(nil)
	if MigrateLegacyKey(&s) {
		changed = true
	}
	if changed && exists {
		_ = core.WriteJSON(SettingsFile(), s)
	}
	return s
}

func SaveSettings(s Settings) error {
	settingsMu.Lock()
	defer settingsMu.Unlock()
	s.SchemaVersion = store.Version
	return core.WriteJSON(SettingsFile(), s)
}

// ProjectSettings: the subset a project may override.
type ProjectSettings struct {
	store.Versioned
	Model          string            `json:"model,omitempty"`
	Mode           string            `json:"mode,omitempty"`
	Permissions    map[string]string `json:"permissions,omitempty"`
	ProtectedPaths []string          `json:"protectedPaths,omitempty"`
	Env            map[string]string `json:"env,omitempty"` // for Bash and hooks; never reaches the model
	OutputStyle    string            `json:"outputStyle,omitempty"`
}

// EffectiveSettings merges the layers (layers.go) for a project.
func EffectiveSettings(root string) Settings {
	s := LoadSettings()
	for _, file := range []string{ProjectSettingsFile(root), LocalSettingsFile(root)} {
		var p ProjectSettings
		if readLayer(file, &p) {
			applyProjectLayer(&s, p)
		}
	}
	var managed ProjectSettings
	if readLayer(ManagedSettingsPath(), &managed) {
		applyManaged(&s, managed)
	}
	return s
}

// modeRank orders permission modes from the most restricted to the least.
func modeRank(m string) int {
	switch m {
	case "plan":
		return 0
	case "default", "":
		return 1
	case "acceptEdits":
		return 2
	case "bypass":
		return 3
	}
	return 1
}

// permRank orders a tool permission from the most restricted to the least.
func permRank(v string) int {
	switch v {
	case "deny":
		return 0
	case "ask", "":
		return 1
	case "allow":
		return 2
	}
	return 1
}

// RootLanguage is the language the user reads infuse in, for code that has a project folder but no
// session.
func RootLanguage(root string) string {
	if l := EffectiveSettings(root).Language; l != "" {
		return l
	}
	return "pt-BR"
}
