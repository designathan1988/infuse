package main

// Temporary aliases for the extraction of internal packages. They keep the root package compiling
// while symbols move out, and are removed once the last domain leaves (stage 9).

import (
	"infuse/internal/config"
	"infuse/internal/core"
)

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
