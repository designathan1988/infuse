package api

import (
	"context"
	"infuse/internal/agent"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/knowledge"
	"infuse/internal/mcp"
	"infuse/internal/oracle"
	"infuse/internal/platform/process"
	"infuse/internal/workspace"
	"os/exec"
	"time"
)

func iconPNG(size int) []byte { return desktop.IconPNG(size) }
func RunWindow(url string)    { desktop.RunWindow(url) }
func osDialogs(owner func() uintptr) desktop.NativeDialogs {
	return desktop.OSDialogs(owner)
}
func attachmentOf(p string) (Attachment, bool)           { return desktop.AttachmentOf(p) }
func gitRun(root string, args ...string) (string, error) { return workspace.GitRun(root, args...) }
func gitOutput(root string, args ...string) string       { return workspace.GitOutput(root, args...) }

type OracleCheck = oracle.OracleCheck
type OracleSpec = oracle.OracleSpec
type CheckResult = oracle.CheckResult
type Verdict = oracle.Verdict
type CheckSuggestion = oracle.CheckSuggestion

func LoadOracleSpec(root string) (OracleSpec, error) { return oracle.LoadOracleSpec(root) }
func RunOracle(ctx context.Context, evidenceRoot, workDir, session string) (Verdict, error) {
	return oracle.RunOracle(ctx, evidenceRoot, workDir, session)
}
func ListVerdicts(root string, n int) []Verdict   { return oracle.ListVerdicts(root, n) }
func oracleSpecFile(root string) string           { return oracle.SpecFile(root) }
func evidenceDir(root string) string              { return oracle.EvidenceDir(root) }
func SuggestChecks(root string) []CheckSuggestion { return oracle.SuggestChecks(root) }

type mcpServerConfig = mcp.ServerConfig
type mcpStatus = mcp.Status

func mcpFor(root string) *mcp.Manager { return mcp.For(root) }
func mcpToolName(server, tool string) string {
	return mcp.ToolName(server, tool)
}
func editMCP(root, scope string, f func(map[string]mcpServerConfig)) error {
	return mcp.Edit(root, scope, f)
}
func closeAllMCP() { mcp.CloseAll() }

type ConventionRules = knowledge.ConventionRules
type Tally = knowledge.Tally
type ProjectRules = knowledge.ProjectRules
type compiledRule = knowledge.CompiledRule
type compiledRules = knowledge.CompiledRules

var instructionNames = knowledge.InstructionNames
var projectRulesSpec = knowledge.ProjectRulesSpec

const compilePrompt = knowledge.CompilePrompt

func loadProjectRules(root string) ProjectRules { return knowledge.LoadProjectRules(root) }
func gitHeadFile(root, rel string) string       { return knowledge.GitHeadFile(root, rel) }
func resolveInstructionImports(root, dir, text string, depth int) string {
	return knowledge.ResolveInstructionImports(root, dir, text, depth)
}
func isInstructionFile(rel string) bool { return knowledge.IsInstructionFile(rel) }
func underReference(r ProjectRules, rel string) string {
	return knowledge.UnderReference(r, rel)
}
func verifyRule(root, normText string, r CompiledRule) Msg {
	return knowledge.VerifyRule(root, normText, r)
}

func ConventionsFile(root string) string     { return knowledge.ConventionsFile(root) }
func conventionsFile(root string) string     { return knowledge.ConventionsFile(root) }
func ConventionsPrompt(root string) string   { return knowledge.ConventionsPrompt(root) }
func conventionsPrompt(root string) string   { return knowledge.ConventionsPrompt(root) }
func LearnConventions(root string) string    { return knowledge.LearnConventions(root) }
func conventionRulesFile(root string) string { return knowledge.ConventionRulesFile(root) }
func loadConventionRules(root string) *ConventionRules {
	return knowledge.LoadConventionRules(root)
}

type launchConfig = workspace.LaunchConfig
type errLaunchNeedsApproval = workspace.ErrLaunchNeedsApproval
type fileChange = workspace.FileChange
type editorApp = workspace.Editor

func CreateWorktree(root, name string) (string, string, error) {
	return workspace.CreateWorktree(root, name)
}
func RemoveWorktree(root, dir string, force bool) error {
	return workspace.RemoveWorktree(root, dir, force)
}
func DevServers(root string) []map[string]any          { return workspace.DevServers(root) }
func StartDevServer(root, name string) error           { return workspace.StartDevServer(root, name) }
func StopDevServer(root, name string)                  { workspace.StopDevServer(root, name) }
func launchConfigs(root string) []launchConfig         { return workspace.LaunchConfigs(root) }
func approveLaunch(root string, c launchConfig) error  { return workspace.ApproveLaunch(root, c) }
func launchApproved(root string, c launchConfig) bool  { return workspace.LaunchApproved(root, c) }
func launchCommandLine(c launchConfig) string          { return workspace.LaunchCommandLine(c) }
func ghRun(dir string, args ...string) (string, error) { return workspace.GhRun(dir, args...) }

var errConflict = workspace.ErrConflict
var snapSkipDirs = workspace.SnapSkipDirs

func Changes(dir string) map[string]any   { return workspace.Changes(dir) }
func PRStatus(dir string) map[string]any  { return workspace.PRStatus(dir) }
func CreatePR(dir string) (string, error) { return workspace.CreatePR(dir) }
func Editors() []editorApp                { return workspace.Editors() }
func OpenInEditor(id, path string) error  { return workspace.OpenInEditor(id, path) }
func WriteUserFile(root, rel, content string, mtime int64, force bool) (int64, error) {
	return workspace.WriteUserFile(root, rel, content, mtime, force)
}
func FailedCILogs(dir string) (string, error) { return workspace.FailedCILogs(dir) }
func snapshotTurn(root, work, session string, turn int) {
	workspace.SnapshotTurn(root, work, session, turn)
}

type rootSnapshots struct{}

func (rootSnapshots) LiveSessions(root string) []workspace.LiveSession {
	return agent.LiveSessions(root)
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
func shellQuote(s string) string    { return filter.ShellQuote(s) }
func detach(cmd *exec.Cmd)          { process.Detach(cmd) }
func noConsoleWindow(cmd *exec.Cmd) { process.NoConsoleWindow(cmd) }
func attachParentConsole()          { process.AttachParentConsole() }
func processAlive(pid int) bool     { return process.ProcessAlive(pid) }
func procGroup(cmd *exec.Cmd)       { process.ProcGroup(cmd) }
func attachJob(cmd *exec.Cmd)       { process.AttachJob(cmd) }
func killTree(cmd *exec.Cmd)        { process.KillTree(cmd) }
func releaseJob(cmd *exec.Cmd)      { process.ReleaseJob(cmd) }

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
		Memory:           []string{knowledge.MemoryRel},
	}
}

func newGuard(root string, protected []string) *filter.Guard {
	return filter.NewGuard(root, guardPolicy(root, protected))
}
func RunShell(args []string) int                       { return filter.RunShell(args, guardPolicy) }
func TransformCommand(cmd, cwd string) TransformResult { return filter.TransformCommand(cmd, cwd) }
func FindHeredocs(cmd string) []Heredoc                { return filter.FindHeredocs(cmd) }
func DetectFileWrite(cmd string) *FileWriteIntent      { return filter.DetectFileWrite(cmd) }
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
