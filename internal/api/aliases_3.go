package api

import (
	"context"
	"infuse/internal/agent"
	"infuse/internal/browser"
	"infuse/internal/connections"
	"infuse/internal/filter"
)

func FileDiagnostics(root, abs string) string { return filter.FileDiagnostics(root, abs) }
func fileDiagnostics(root, abs string) string { return filter.FileDiagnostics(root, abs) }
func BuildTrailer(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	return filter.BuildTrailer(root, cwd, original, actions, started, rules, ruleLines)
}
func TrailerMsgs(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, notes []Msg) []Msg {
	return filter.TrailerMsgs(root, cwd, original, actions, started, rules, notes)
}
func buildTrailer(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	return filter.BuildTrailer(root, cwd, original, actions, started, rules, ruleLines)
}
func trailerMsgs(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, notes []Msg) []Msg {
	return filter.TrailerMsgs(root, cwd, original, actions, started, rules, notes)
}
func ListedDirectory(cmd string) (string, bool) { return filter.ListedDirectory(cmd) }
func MayChangeTree(cmd string) bool             { return filter.MayChangeTree(cmd) }

func Install() InstallResult                    { return filter.Install() }
func Uninstall() InstallResult                  { return filter.Uninstall() }
func GetStatus() Status                         { return filter.GetStatus() }
func printJSON(v any)                           { filter.PrintJSON(v) }
func msgStrings(ms []Msg) []string              { return filter.MsgStrings(ms) }
func resolveIn(cwd, p string) string            { return filter.ResolveIn(cwd, p) }
func ignoredPath(root, p string) bool           { return filter.IgnoredPath(root, p) }
func projectRulesFile(root string) string       { return filter.ProjectRulesFile(root) }
func globalRulesFile() string                   { return filter.GlobalRulesFile() }
func listedDirectory(cmd string) (string, bool) { return filter.ListedDirectory(cmd) }
func mayChangeTree(cmd string) bool             { return filter.MayChangeTree(cmd) }

var reRedir = filter.ReRedir

// Agent runtime surface kept for the API during the migration.

type Session = agent.Session
type SessionData = agent.SessionData
type SessionHit = agent.SessionHit
type Mission = agent.Mission
type MissionStep = agent.MissionStep
type Diagnosis = agent.Diagnosis
type Evidence = agent.Evidence
type AgentRun = agent.AgentRun
type AgentStep = agent.AgentStep
type ReportClaim = agent.ReportClaim
type ReportSource = agent.ReportSource
type TurnReport = agent.TurnReport
type Ban = agent.Ban
type HookEntry = agent.HookEntry
type CompiledRule = agent.CompiledRule
type PinnedDoc = agent.PinnedDoc
type MemoryItem = agent.MemoryItem
type Skill = agent.Skill
type SearchResult = agent.SearchResult
type docSuggestion = agent.DocSuggestion

func newSession(root string) *Session               { return agent.NewSession(root) }
func loadSession(root, id string) (*Session, error) { return agent.LoadSession(root, id) }
func listSessions(root string) []SessionData        { return agent.ListSessions(root) }
func searchSessions(q string, max int) []SessionHit { return agent.SearchSessions(q, max) }
func resumeInterruptedSessions()                    { agent.ResumeInterruptedSessions() }
func sessionsDir(root string) string                { return agent.SessionsDir(root) }
func toolSpecs() []ToolSpec                         { return agent.ToolSpecs() }
func stopAllProcs()                                 { agent.StopAllProcs() }
func loadDocs(root string) []PinnedDoc              { return agent.LoadDocs(root) }
func unpinDoc(root, url string) error               { return agent.UnpinDoc(root, url) }
func docsPrompt(root string) string                 { return agent.DocsPrompt(root) }
func PinDoc(ctx context.Context, root, url string) (PinnedDoc, error) {
	return agent.PinDoc(ctx, root, url)
}
func SuggestDocs(root string) []docSuggestion { return agent.SuggestDocs(root) }
func addHook(root, scope, event, matcher, command string) error {
	return agent.AddHook(root, scope, event, matcher, command)
}
func removeHook(root, key string) error { return agent.RemoveHook(root, key) }
func LoadHooks(root string) []HookEntry { return agent.LoadHooks(root) }
func WebSearch(ctx context.Context, q string, max int) ([]SearchResult, string, error) {
	return agent.WebSearch(ctx, q, max)
}

var hookEvents = agent.HookEvents

func hookKey(root, scope, event, matcher, command string) string {
	return agent.HookKey(root, scope, event, matcher, command)
}
func approveHook(key string) error                     { return agent.ApproveHook(key) }
func parseSkill(text string) (string, string, string)  { return agent.ParseSkill(text) }
func LoadSkills(root string) []Skill                   { return agent.LoadSkills(root) }
func projectMemoryFile(root string) string             { return agent.ProjectMemoryFile(root) }
func globalMemoryFile() string                         { return agent.GlobalMemoryFile() }
func loadMemory(file string) []MemoryItem              { return agent.LoadMemory(file) }
func saveMemory(file string, items []MemoryItem) error { return agent.SaveMemory(file, items) }
func normalizeMemory(it *MemoryItem) error             { return agent.NormalizeMemory(it) }

func openDefaultBrowser(u string) error            { return agent.OpenDefaultBrowser(u) }
func frameable(ctx context.Context, u string) bool { return agent.Frameable(ctx, u) }

func readIfExists(p string) string        { return agent.ReadIfExists(p) }
func loadInstructions(root string) string { return agent.LoadInstructions(root) }
func closeAgentBrowsers()                 { browser.CloseAgentBrowsers() }

type ModelChoice = connections.ModelChoice

func ConnectProvider(ctx context.Context, id, key, base string) (Connection, error) {
	return connections.ConnectProvider(ctx, id, key, base)
}
func DisconnectProvider(id string) error       { return connections.DisconnectProvider(id) }
func AvailableModels(s Settings) []ModelChoice { return connections.AvailableModels(s) }
func PreferredModel(models []string) string    { return connections.PreferredModel(models) }
func connectionConfig(s Settings, id string) (string, ProviderConfig) {
	return connections.ConnectionConfig(s, id)
}
func providerFor(s Settings, id string) (Provider, error) { return connections.ProviderFor(s, id) }
