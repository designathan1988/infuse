package main

// Temporary aliases for the extraction of internal packages. They keep the root package compiling
// while symbols move out, and are removed once the last domain leaves (stage 9).

import (
	"context"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/llm"
	"infuse/internal/projectmap"
	"infuse/internal/security"
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
