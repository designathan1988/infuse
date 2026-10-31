package agent

// Internal aliases used while the runtime is one package: the data types and helpers it shares with
// the rest of the application. They keep the agent code focused on its own domain.

import (
	"context"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/filter"
	"infuse/internal/journal"
	"infuse/internal/knowledge"
	"infuse/internal/lang"
	"infuse/internal/llm"
	"infuse/internal/mcp"
	"infuse/internal/oracle"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"infuse/internal/security"
	"infuse/internal/workspace"
	"os/exec"
	"regexp"
)

type (
	Block          = core.Block
	Message        = core.Message
	Usage          = core.Usage
	ToolSpec       = core.ToolSpec
	TaskItem       = core.TaskItem
	Event          = core.Event
	CheckpointFile = core.CheckpointFile
	Attachment     = core.Attachment
	InlineImage    = core.InlineImage
	Msg            = core.Msg

	Settings       = config.Settings
	Provider       = llm.Provider
	ProviderConfig = llm.ProviderConfig
	ModelRequest   = llm.ModelRequest
	ModelResult    = llm.ModelResult
	StreamEvent    = llm.StreamEvent

	Guard           = filter.Guard
	Heredoc         = filter.Heredoc
	FileWriteIntent = filter.FileWriteIntent
	Rule            = filter.Rule
	RulesFile       = filter.RulesFile
	RuleSet         = filter.RuleSet
	TransformResult = filter.TransformResult

	JournalEntry = journal.JournalEntry
	LessonEntry  = journal.LessonEntry

	ProjectMap = projectmap.ProjectMap
	FileInfo   = projectmap.FileInfo
	DirInfo    = projectmap.DirInfo
	Hotspot    = projectmap.Hotspot

	ConventionRules  = knowledge.ConventionRules
	ProjectRules     = knowledge.ProjectRules
	Tally            = knowledge.Tally
	OracleCheck      = oracle.OracleCheck
	OracleSpec       = oracle.OracleSpec
	CheckResult      = oracle.CheckResult
	Verdict          = oracle.Verdict
	CheckSuggestion  = oracle.CheckSuggestion
)

const (
	infuseDirName   = core.InfuseDirName
	askJSONAttempts = llm.AskJSONAttempts
	fallbackContext = llm.FallbackContext
	compilePrompt   = knowledge.CompilePrompt
)

var (
	instructionNames = knowledge.InstructionNames
	ignoredDirs      = projectmap.IgnoredDirs
	snapSkipDirs     = workspace.SnapSkipDirs
)

func M(id string, kv ...any) Msg                { return core.M(id, kv...) }
func msgText(m Message) string                  { return core.TextOf(m) }
func truncate(s string, n int) string           { return core.Truncate(s, n) }
func firstLine(s string) string                 { return core.FirstLine(s) }
func firstNonEmpty(a, b string) string          { return core.FirstNonEmpty(a, b) }
func normSpace(s string) string                 { return core.NormSpace(s) }
func splitLines(s string) []string              { return core.SplitLines(s) }
func snippetAround(text string, k, n int) string { return core.SnippetAround(text, k, n) }
func dedupe(xs []string) []string               { return core.Dedupe(xs) }
func contains(list []string, v string) bool     { return core.Contains(list, v) }
func firstNonZero(a, b float64) float64         { return core.FirstNonZero(a, b) }
func sortedKeys[M ~map[string]V, V any](m M) []string { return core.SortedKeys(m) }
func exists(p string) bool                      { return core.Exists(p) }
func readJSON(file string, v any) error         { return core.ReadJSON(file, v) }
func writeJSON(file string, v any) error        { return core.WriteJSON(file, v) }
func within(root, p string) bool                { return core.Within(root, p) }
func toPosix(p string) string                   { return core.ToPosix(p) }
func relTo(root, abs string) string             { return core.RelTo(root, abs) }
func FromMsysPath(p string) string              { return core.FromMsysPath(p) }
func InfuseHome() string                        { return core.InfuseHome() }
func randomHex(n int) string                    { return core.RandomHex(n) }
func exitCode(err error) int                    { return core.ExitCode(err) }
func errMsg(m Msg) error                        { return core.ErrMsg(m) }
func userError(lang string, m Msg) error        { return core.UserError(lang, m) }
func msgOf(err error) (Msg, bool)               { return core.MsgOf(err) }
func errArg(err error) any                      { return core.ErrArg(err) }
func displayFor(lang, content string, msgs []Msg) string { return core.DisplayFor(lang, content, msgs) }
func globToRegex(pat string) *regexp.Regexp     { return lang.GlobToRegex(pat) }
func FindRealBash() string                      { return core.FindRealBash() }
func noConsoleWindow(cmd *exec.Cmd)             { process.NoConsoleWindow(cmd) }
func gitRun(root string, args ...string) (string, error) { return workspace.GitRun(root, args...) }

func LoadSettings() Settings                    { return config.LoadSettings() }
func SaveSettings(s Settings) error             { return config.SaveSettings(s) }
func EffectiveSettings(root string) Settings    { return config.EffectiveSettings(root) }
func recordUsage(e usageEntry)                  { config.RecordUsage(e) }
func todaySpend() float64                       { return config.TodaySpend() }
func rootLang(root string) string               { return config.RootLanguage(root) }

func newProvider(name string, cfg ProviderConfig) (Provider, error) { return llm.NewProvider(name, cfg) }
func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	return llm.ListModels(ctx, name, cfg)
}
func askJSON(ctx context.Context, p Provider, model, system, user string, spec ToolSpec, maxTokens int, out any) (Usage, error) {
	return llm.AskJSON(ctx, p, model, system, user, spec, maxTokens, out)
}
func repairTranscript(msgs []Message) []Message { return llm.RepairTranscript(msgs) }
func contextWindow(model string) int            { return llm.ContextWindow(model) }
func outputLimit(model string, configured int) int { return llm.OutputLimit(model, configured) }

func LoadRules(root string) *RuleSet { return filter.LoadRules(root) }
func loadProjectRules(root string) ProjectRules { return knowledge.LoadProjectRules(root) }
func isInstructionFile(rel string) bool { return knowledge.IsInstructionFile(rel) }
func fileDiagnosticsX() {}
func gitHeadFile(root, rel string) string { return knowledge.GitHeadFile(root, rel) }
func resolveInstructionImports(root, dir, text string, depth int) string {
	return knowledge.ResolveInstructionImports(root, dir, text, depth)
}
func homeDir() string { return core.HomeDir() }
func readJSONL(file string) []map[string]any { return core.ReadJSONL(file) }
func underReference(r ProjectRules, rel string) string { return knowledge.UnderReference(r, rel) }
var version = core.Version
func procGroup(cmd *exec.Cmd) { process.ProcGroup(cmd) }
func resolveIn(cwd, p string) string { return filter.ResolveIn(cwd, p) }
func trailerMsgs(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, notes []Msg) []Msg {
	return filter.TrailerMsgs(root, cwd, original, actions, started, rules, notes)
}
func ignoredPath(root, p string) bool { return filter.IgnoredPath(root, p) }
type capWriter = core.CapWriter
func newCapWriter(limit int) *capWriter { return core.NewCapWriter(limit) }
const (
	ActLesson         = filter.ActLesson
	ActHeredocToFile  = filter.ActHeredocToFile
	ActHeredocToStdin = filter.ActHeredocToStdin
)
func loadProjects() []config.ProjectEntry { return config.LoadProjects() }
func RememberProject(root string) { config.RememberProject(root) }
func snapshotTurn(root, work, session string, turn int) { workspace.SnapshotTurn(root, work, session, turn) }
func msgStrings(ms []Msg) []string { return filter.MsgStrings(ms) }
func hiddenInternal(rel string) bool { return filter.HiddenInternal(rel) }
func fileDiagnostics(root, abs string) string { return filter.FileDiagnostics(root, abs) }
var imageExt = map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp"}
func killTree(cmd *exec.Cmd) { process.KillTree(cmd) }
func attachJob(cmd *exec.Cmd) { process.AttachJob(cmd) }
func releaseJob(cmd *exec.Cmd) { process.ReleaseJob(cmd) }
func processAlive(pid int) bool { return process.ProcessAlive(pid) }
func detach(cmd *exec.Cmd) { process.Detach(cmd) }

func newGuard(root string, protected []string) *filter.Guard {
	return filter.NewGuard(root, filter.Policy{
		Protected:        protected,
		References:       loadProjectRules(root).Reference,
		InstructionFiles: instructionNames,
		Memory:           []string{knowledge.MemoryRel},
	})
}
func TransformCommand(cmd, cwd string) TransformResult { return filter.TransformCommand(cmd, cwd) }
func FindHeredocs(cmd string) []Heredoc                { return filter.FindHeredocs(cmd) }
func DetectFileWrite(cmd string) *FileWriteIntent      { return filter.DetectFileWrite(cmd) }
func CommandLineOf(cmd string, h Heredoc) (int, int, string) { return filter.CommandLineOf(cmd, h) }
func HiddenInternal(rel string) bool                   { return filter.HiddenInternal(rel) }
func ApplyCommandRulesMsgs(root string, set *RuleSet, command string) (string, []Msg) {
	return filter.ApplyCommandRulesMsgs(root, set, command)
}
func ApplyWriteRulesMsgs(root string, set *RuleSet, files []string) []Msg {
	return filter.ApplyWriteRulesMsgs(root, set, files)
}
func ApplyListRulesMsgs(root string, set *RuleSet, dir string) []Msg {
	return filter.ApplyListRulesMsgs(root, set, dir)
}
func FileDiagnostics(root, abs string) string   { return filter.FileDiagnostics(root, abs) }
func TrailerMsgs(root, cwd, original string, actions []filter.TransformAction, started int64, rules *RuleSet, notes []Msg) []Msg {
	return filter.TrailerMsgs(root, cwd, original, actions, started, rules, notes)
}

func Journal(root string, e JournalEntry) { journal.Journal(root, e) }
func Lesson(root, vice, detail string)    { journal.Lesson(root, vice, detail) }
func infuseDir(root string) string        { return journal.InfuseDir(root) }
func ReadJournalSince(root string, since int64) []JournalEntry {
	return journal.ReadJournalSince(root, since)
}

func BuildMap(root string) *ProjectMap    { return projectmap.BuildMap(root) }
func WriteMap(root string, m *ProjectMap) { projectmap.WriteMap(root, m) }
func LoadMap(root string) *ProjectMap     { return projectmap.LoadMap(root) }
func LoadOrBuildMap(root string) *ProjectMap { return projectmap.LoadOrBuildMap(root) }
func DescribeDir(m *ProjectMap, relDir string) string { return projectmap.DescribeDir(m, relDir) }
func DependentsMsg(m *ProjectMap, relFile string) Msg { return projectmap.DependentsMsg(m, relFile) }

func conventionsFile(root string) string   { return knowledge.ConventionsFile(root) }
func conventionsPrompt(root string) string { return knowledge.ConventionsPrompt(root) }
func loadConventionRules(root string) *ConventionRules { return knowledge.LoadConventionRules(root) }
func conventionRulesFile(root string) string { return knowledge.ConventionRulesFile(root) }

func RunOracle(ctx context.Context, evidenceRoot, workDir, session string) (Verdict, error) {
	return oracle.RunOracle(ctx, evidenceRoot, workDir, session)
}
func CreateWorktree(root, name string) (string, string, error) { return workspace.CreateWorktree(root, name) }
func redactSecrets(s string) string { return security.RedactSecrets(s) }
func externalContent(source, text string) string { return security.ExternalContent(source, text) }
func privateTargetMsg(raw string) (bool, Msg) { return security.PrivateTargetMsg(raw) }

func mcpFor(root string) *mcp.Manager { return mcp.For(root) }

func connectionConfig(s Settings, id string) (string, ProviderConfig) {
	p := config.PresetFor(id)
	c := s.Connections[id]
	kind := c.Kind
	if kind == "" {
		kind = p.Kind
	}
	base := c.BaseURL
	if base == "" {
		base = p.BaseURL
	}
	key, _ := config.SecretGet(config.SecretName(id))
	return kind, ProviderConfig{APIKey: key, BaseURL: base}
}

func providerFor(s Settings, id string) (Provider, error) {
	kind, cfg := connectionConfig(s, id)
	return llm.NewProvider(kind, cfg)
}

var blockMsgs = map[string]bool{
	"protocol.no_plan": true, "protocol.docs_first": true, "protocol.convention": true, "protocol.file_grows": true,
	"protocol.file_too_big": true, "gate.research_local": true, "gate.research_web": true, "instr.required": true,
	"instr.no_edit": true, "instr.reference": true, "instr.loop": true, "instr.loop_docs": true,
	"gate.redo": true, "gate.no_diagnosis": true,
}

func (tc *ToolContext) SayErr(err error) string {
	if m, ok := msgOf(err); ok {
		return tc.Say(m)
	}
	return err.Error()
}

type loopBreaker struct {
	same        map[string]int
	consecutive int
}

func (b *loopBreaker) record(name string, input json.RawMessage, isErr bool) error {
	if !isErr {
		b.consecutive = 0
		return nil
	}
	if b.same == nil {
		b.same = map[string]int{}
	}
	key := name + "|" + string(input)
	b.same[key]++
	b.consecutive++
	if b.same[key] >= 3 {
		return errMsg(M("breaker.same", "tool", name))
	}
	if b.consecutive >= 8 {
		return errMsg(M("breaker.many"))
	}
	return nil
}

type usageEntry = config.UsageEntry

