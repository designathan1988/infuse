package main

// Temporary aliases for the extraction of internal packages. They keep the root package compiling
// while symbols move out, and are removed once the last domain leaves (stage 9).

import (
	"context"
	"infuse/internal/config"
	"infuse/internal/filter"
	"regexp"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"infuse/internal/llm"
	"infuse/internal/oracle"
	"infuse/internal/projectmap"
	"infuse/internal/workspace"
	"infuse/internal/platform/process"
	"infuse/internal/security"
	"os/exec"
	"time"
)

type JournalEntry = journal.JournalEntry
type LessonEntry = journal.LessonEntry
type ProjectMap = projectmap.ProjectMap
type FileInfo = projectmap.FileInfo
type DirInfo = projectmap.DirInfo
type Hotspot = projectmap.Hotspot

var ignoredDirs = projectmap.IgnoredDirs

func infuseDir(root string) string          { return journal.InfuseDir(root) }
func Journal(root string, e JournalEntry)   { journal.Journal(root, e) }
func Lesson(root, vice, detail string)      { journal.Lesson(root, vice, detail) }
func ReadJournalSince(root string, since int64) []JournalEntry {
	return journal.ReadJournalSince(root, since)
}
func BuildMap(root string) *ProjectMap                  { return projectmap.BuildMap(root) }
func WriteMap(root string, m *ProjectMap)               { projectmap.WriteMap(root, m) }
func LoadMap(root string) *ProjectMap                   { return projectmap.LoadMap(root) }
func LoadOrBuildMap(root string) *ProjectMap            { return projectmap.LoadOrBuildMap(root) }
func DescribeDir(m *ProjectMap, relDir string) string   { return projectmap.DescribeDir(m, relDir) }
func DescribeDependents(m *ProjectMap, relFile string) string {
	return projectmap.DescribeDependents(m, relFile)
}
func DependentsMsg(m *ProjectMap, relFile string) Msg { return projectmap.DependentsMsg(m, relFile) }
func mapFile(root string) string                       { return projectmap.MapFile(root) }
func appendLine(file string, v any)                    { core.AppendLine(file, v) }
func readJSONL(file string) []map[string]any           { return core.ReadJSONL(file) }

type capWriter = core.CapWriter

func newCapWriter(limit int) *capWriter       { return core.NewCapWriter(limit) }
func redactSecrets(s string) string           { return security.RedactSecrets(s) }
func privateTarget(raw string) (bool, string) { return security.PrivateTarget(raw) }
func privateTargetMsg(raw string) (bool, Msg) { return security.PrivateTargetMsg(raw) }
func externalContent(source, text string) string {
	return security.ExternalContent(source, text)
}
func todaySpend() float64 { return config.TodaySpend() }

type Provider = llm.Provider
type ProviderConfig = llm.ProviderConfig
type ModelRequest = llm.ModelRequest
type ModelResult = llm.ModelResult
type StreamEvent = llm.StreamEvent

const askJSONAttempts = llm.AskJSONAttempts
const fallbackContext = llm.FallbackContext

func newProvider(name string, cfg ProviderConfig) (Provider, error) { return llm.NewProvider(name, cfg) }
func registerProvider(name string, mk func(cfg ProviderConfig) Provider) {
	llm.RegisterProvider(name, mk)
}
func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	return llm.ListModels(ctx, name, cfg)
}
func askJSON(ctx context.Context, p Provider, model, system, user string, spec ToolSpec, maxTokens int, out any) (Usage, error) {
	return llm.AskJSON(ctx, p, model, system, user, spec, maxTokens, out)
}
func repairTranscript(msgs []Message) []Message { return llm.RepairTranscript(msgs) }
func contextWindow(model string) int            { return llm.ContextWindow(model) }
func outputLimit(model string, configured int) int {
	return llm.OutputLimit(model, configured)
}
func providerNames() []string { return llm.ProviderNames() }

const infuseDirName = core.InfuseDirName

type Settings = config.Settings
type projectEntry = config.ProjectEntry

func projectsFile() string                 { return config.ProjectsFile() }
func loadProjects() []projectEntry         { return config.LoadProjects() }
func saveProjects(list []projectEntry) error { return config.SaveProjects(list) }
func RememberProject(root string)          { config.RememberProject(root) }
type ProjectSettings = config.ProjectSettings
type Connection = config.Connection
type providerPreset = config.ProviderPreset
type secretStore = config.SecretStore
type usageEntry = config.UsageEntry
type usageTotals = config.Totals
type dayTotal = config.DayTotal

var providerPresets = config.ProviderPresets

func LoadSettings() Settings                    { return config.LoadSettings() }
func SaveSettings(s Settings) error             { return config.SaveSettings(s) }
func EffectiveSettings(root string) Settings    { return config.EffectiveSettings(root) }
func presetFor(id string) providerPreset        { return config.PresetFor(id) }
func secretName(id string) string               { return config.SecretName(id) }
func keyHint(k string) string                   { return config.KeyHint(k) }
func secretGet(name string) (string, error)     { return config.SecretGet(name) }
func secretSet(name, v string) error            { return config.SecretSet(name, v) }
func secretDelete(name string) error            { return config.SecretDelete(name) }
func recordUsage(e usageEntry)                  { config.RecordUsage(e) }
func UsageTotals() usageTotals                  { return config.UsageTotals() }
func usageFile() string                         { return config.UsageFile() }
func settingsFile() string                      { return config.SettingsFile() }
func msgText(m Message) string                  { return core.TextOf(m) }
func errMsg(m Msg) error                        { return core.ErrMsg(m) }
func userError(lang string, m Msg) error        { return core.UserError(lang, m) }
func msgOf(err error) (Msg, bool)               { return core.MsgOf(err) }
func errArg(err error) any                      { return core.ErrArg(err) }
func selfPath() string                          { return core.SelfPath() }
func globToRegex(pat string) *regexp.Regexp     { return lang.GlobToRegex(pat) }

var version = core.Version

func homeDir() string                  { return core.HomeDir() }
func InfuseHome() string               { return core.InfuseHome() }
func FindRoot(cwd string) string       { return core.FindRoot(cwd) }
func FindRealBash() string             { return core.FindRealBash() }
func FromMsysPath(p string) string     { return core.FromMsysPath(p) }
func toBashPath(p string) string       { return core.ToBashPath(p) }
func toPosix(p string) string          { return core.ToPosix(p) }
func exists(p string) bool             { return core.Exists(p) }
func readJSON(file string, v any) error { return core.ReadJSON(file, v) }
func writeJSON(file string, v any) error { return core.WriteJSON(file, v) }
func within(root, p string) bool       { return core.Within(root, p) }
func relTo(root, abs string) string    { return core.RelTo(root, abs) }

type Block = core.Block
type Message = core.Message
type ToolSpec = core.ToolSpec
type Usage = core.Usage
type InlineImage = core.InlineImage
type TaskItem = core.TaskItem
type CheckpointFile = core.CheckpointFile
type Event = core.Event
type Attachment = core.Attachment
type Msg = core.Msg

func M(id string, kv ...any) Msg { return core.M(id, kv...) }

func registerMessages(m map[string]map[string]string) { core.RegisterMessages(m) }

func displayFor(lang, content string, msgs []Msg) string {
	return core.DisplayFor(lang, content, msgs)
}

func truncate(s string, n int) string              { return core.Truncate(s, n) }
func firstLine(s string) string                    { return core.FirstLine(s) }
func firstNonEmpty(a, b string) string             { return core.FirstNonEmpty(a, b) }
func normSpace(s string) string                    { return core.NormSpace(s) }
func splitLines(s string) []string                 { return core.SplitLines(s) }
func snippetAround(text string, k, n int) string   { return core.SnippetAround(text, k, n) }
func dedupe(xs []string) []string                  { return core.Dedupe(xs) }
func contains(list []string, v string) bool        { return core.Contains(list, v) }
func firstNonZero(a, b float64) float64            { return core.FirstNonZero(a, b) }
func sortedKeys[M ~map[string]V, V any](m M) []string { return core.SortedKeys(m) }
func exitCode(err error) int                         { return core.ExitCode(err) }
func randomHex(n int) string                         { return core.RandomHex(n) }
func gitRun(root string, args ...string) (string, error) { return workspace.GitRun(root, args...) }

type OracleCheck = oracle.OracleCheck
type OracleSpec = oracle.OracleSpec
type CheckResult = oracle.CheckResult
type Verdict = oracle.Verdict
type CheckSuggestion = oracle.CheckSuggestion

func LoadOracleSpec(root string) (OracleSpec, error) { return oracle.LoadOracleSpec(root) }
func RunOracle(ctx context.Context, evidenceRoot, workDir, session string) (Verdict, error) {
	return oracle.RunOracle(ctx, evidenceRoot, workDir, session)
}
func ListVerdicts(root string, n int) []Verdict { return oracle.ListVerdicts(root, n) }
func oracleSpecFile(root string) string         { return oracle.SpecFile(root) }
func evidenceDir(root string) string            { return oracle.EvidenceDir(root) }
func SuggestChecks(root string) []CheckSuggestion { return oracle.SuggestChecks(root) }

type launchConfig = workspace.LaunchConfig
type errLaunchNeedsApproval = workspace.ErrLaunchNeedsApproval
type fileChange = workspace.FileChange
type editorApp = workspace.Editor

func CreateWorktree(root, name string) (string, string, error) { return workspace.CreateWorktree(root, name) }
func RemoveWorktree(root, dir string, force bool) error       { return workspace.RemoveWorktree(root, dir, force) }
func DevServers(root string) []map[string]any                 { return workspace.DevServers(root) }
func StartDevServer(root, name string) error                  { return workspace.StartDevServer(root, name) }
func StopDevServer(root, name string)                         { workspace.StopDevServer(root, name) }
func launchConfigs(root string) []launchConfig                { return workspace.LaunchConfigs(root) }
func approveLaunch(root string, c launchConfig) error         { return workspace.ApproveLaunch(root, c) }
func launchApproved(root string, c launchConfig) bool         { return workspace.LaunchApproved(root, c) }
func launchCommandLine(c launchConfig) string                 { return workspace.LaunchCommandLine(c) }
func ghRun(dir string, args ...string) (string, error)        { return workspace.GhRun(dir, args...) }

var errConflict = workspace.ErrConflict
var snapSkipDirs = workspace.SnapSkipDirs
func Changes(dir string) map[string]any                       { return workspace.Changes(dir) }
func PRStatus(dir string) map[string]any                      { return workspace.PRStatus(dir) }
func CreatePR(dir string) (string, error)                     { return workspace.CreatePR(dir) }
func Editors() []editorApp                                    { return workspace.Editors() }
func OpenInEditor(id, path string) error                      { return workspace.OpenInEditor(id, path) }
func WriteUserFile(root, rel, content string, mtime int64, force bool) (int64, error) {
	return workspace.WriteUserFile(root, rel, content, mtime, force)
}
func FailedCILogs(dir string) (string, error) { return workspace.FailedCILogs(dir) }
func snapshotTurn(root, work, session string, turn int) {
	workspace.SnapshotTurn(root, work, session, turn)
}

type rootSnapshots struct{}

func (rootSnapshots) LiveSessions(root string) []workspace.LiveSession {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	var out []workspace.LiveSession
	for _, s := range sessions {
		s.mu.Lock()
		if s.Root == root {
			cp := map[int][]CheckpointFile{}
			for t, c := range s.Checkpoints {
				cp[t] = append([]CheckpointFile{}, c...)
			}
			out = append(out, workspace.LiveSession{ID: s.ID, Created: s.Created, Checkpoints: cp})
		}
		s.mu.Unlock()
	}
	return out
}

func LocalChanges(root, work, session, scope string) map[string]any {
	return workspace.LocalChanges(root, work, session, scope, rootSnapshots{})
}
func LocalDiff(root, work, session, scope, p string) string {
	return workspace.LocalDiff(root, work, session, scope, p, rootSnapshots{})
}
func LocalDiscard(root, work, session, scope, p string) error {
	return workspace.LocalDiscard(root, work, session, scope, p, rootSnapshots{})
}
func shellQuote(s string) string                     { return filter.ShellQuote(s) }
func detach(cmd *exec.Cmd)                           { process.Detach(cmd) }
func noConsoleWindow(cmd *exec.Cmd)                  { process.NoConsoleWindow(cmd) }
func attachParentConsole()                           { process.AttachParentConsole() }
func processAlive(pid int) bool                      { return process.ProcessAlive(pid) }
func procGroup(cmd *exec.Cmd)                        { process.ProcGroup(cmd) }
func attachJob(cmd *exec.Cmd)                        { process.AttachJob(cmd) }
func killTree(cmd *exec.Cmd)                         { process.KillTree(cmd) }
func releaseJob(cmd *exec.Cmd)                       { process.ReleaseJob(cmd) }

type Guard = filter.Guard
type Heredoc = filter.Heredoc
type FileWriteIntent = filter.FileWriteIntent
type Rule = filter.Rule
type RulesFile = filter.RulesFile
type RuleSet = filter.RuleSet
type TransformResult = filter.TransformResult

const (
	ActHeredocToFile  = filter.ActHeredocToFile
	ActHeredocToStdin = filter.ActHeredocToStdin
	ActLesson         = filter.ActLesson
)

var ExampleRules = filter.ExampleRules

type InstallResult = filter.InstallResult
type Status = filter.Status

func guardPolicy(root string, protected []string) filter.Policy {
	return filter.Policy{
		Protected:        protected,
		References:       loadProjectRules(root).Reference,
		InstructionFiles: instructionNames,
		Memory:           []string{memoryRel},
	}
}

func newGuard(root string, protected []string) *filter.Guard {
	return filter.NewGuard(root, guardPolicy(root, protected))
}
func RunShell(args []string) int                        { return filter.RunShell(args, guardPolicy) }
func TransformCommand(cmd, cwd string) TransformResult  { return filter.TransformCommand(cmd, cwd) }
func FindHeredocs(cmd string) []Heredoc                 { return filter.FindHeredocs(cmd) }
func DetectFileWrite(cmd string) *FileWriteIntent       { return filter.DetectFileWrite(cmd) }
func CommandLineOf(cmd string, h Heredoc) (int, int, string) {
	return filter.CommandLineOf(cmd, h)
}
func HiddenInternal(rel string) bool      { return filter.HiddenInternal(rel) }
func hiddenInternal(rel string) bool      { return filter.HiddenInternal(rel) }
func LoadRules(root string) *RuleSet      { return filter.LoadRules(root) }
func GlobalRulesFile() string             { return filter.GlobalRulesFile() }
func ProjectRulesFile(root string) string { return filter.ProjectRulesFile(root) }
func ApplyCommandRules(root string, set *RuleSet, command string) (string, []string) {
	return filter.ApplyCommandRules(root, set, command)
}
func ApplyCommandRulesMsgs(root string, set *RuleSet, command string) (string, []Msg) {
	return filter.ApplyCommandRulesMsgs(root, set, command)
}
func ApplyWriteRules(root string, set *RuleSet, files []string) []string {
	return filter.ApplyWriteRules(root, set, files)
}
func ApplyWriteRulesMsgs(root string, set *RuleSet, files []string) []Msg {
	return filter.ApplyWriteRulesMsgs(root, set, files)
}
func ApplyListRules(root string, set *RuleSet, dir string) []string {
	return filter.ApplyListRules(root, set, dir)
}
func ApplyListRulesMsgs(root string, set *RuleSet, dir string) []Msg {
	return filter.ApplyListRulesMsgs(root, set, dir)
}
func EnsureWatcher(root string)                  { filter.EnsureWatcher(root) }
func StopWatcher(root string) bool               { return filter.StopWatcher(root) }
func RunWatcher(root string, idle time.Duration) { filter.RunWatcher(root, idle) }
func watcherAlive(root string) (int, bool)       { return filter.WatcherAlive(root) }
func FileDiagnostics(root, abs string) string    { return filter.FileDiagnostics(root, abs) }
func fileDiagnostics(root, abs string) string    { return filter.FileDiagnostics(root, abs) }
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

func Install() InstallResult   { return filter.Install() }
func Uninstall() InstallResult { return filter.Uninstall() }
func GetStatus() Status        { return filter.GetStatus() }
func printJSON(v any)          { filter.PrintJSON(v) }
func msgStrings(ms []Msg) []string            { return filter.MsgStrings(ms) }
func resolveIn(cwd, p string) string          { return filter.ResolveIn(cwd, p) }
func ignoredPath(root, p string) bool         { return filter.IgnoredPath(root, p) }
func projectRulesFile(root string) string     { return filter.ProjectRulesFile(root) }
func globalRulesFile() string                 { return filter.GlobalRulesFile() }
func listedDirectory(cmd string) (string, bool) { return filter.ListedDirectory(cmd) }
func mayChangeTree(cmd string) bool             { return filter.MayChangeTree(cmd) }

var reRedir = filter.ReRedir
