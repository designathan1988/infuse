package agent

// Plugins: packages of commands, skills, agents, hooks and MCP servers, installed from a git URL into
// .infuse/plugins/<name>. Commands, skills and agents only add text the agent reads. Hooks and MCP
// servers run programs: they stay off until the user approves the plugin, a signed record bound to the
// content of the plugin's tree, so an update or an edit takes the approval away.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/knowledge"
	"infuse/internal/mcp"
	"infuse/internal/platform/gitx"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// PluginManifest is plugin.json.
type PluginManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

// Plugin is one installed package, as the list shows it.
type Plugin struct {
	PluginManifest
	Dir      string   `json:"dir"`
	Source   string   `json:"source"`
	Commit   string   `json:"commit,omitempty"`
	Approved bool     `json:"approved"`
	Runs     []string `json:"runs,omitempty"` // what approving it lets run: hook commands, MCP servers
}

// pluginsDir is where installed plugins live.
func pluginsDir(root string) string { return filepath.Join(root, infuseDirName, "plugins") }

// pluginDirs lists one kind of content (agents, commands, skills) of every installed plugin.
func pluginDirs(root, kind string) []string { return knowledge.PluginContentDirs(root, kind) }

func pluginOrigin(dir string) string { return filepath.Join(dir, ".infuse-origin.json") }

var (
	rePluginName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	rePluginURL  = regexp.MustCompile(`^(https://|ssh://|git@|file://)`)
)

// InstallPlugin clones url and places the package under its manifest's name. Nothing in it runs:
// the clone keeps no hooks, skips submodules, and allows no remote helper protocols.
func InstallPlugin(ctx context.Context, root, url string) (Plugin, error) {
	if !rePluginURL.MatchString(url) || strings.HasPrefix(url, "-") {
		return Plugin{}, errMsg(M("plugin.bad_url", "url", url))
	}
	if err := os.MkdirAll(pluginsDir(root), 0o755); err != nil {
		return Plugin{}, err
	}
	tmp := filepath.Join(pluginsDir(root), ".tmp-"+randomHex(6))
	defer os.RemoveAll(tmp)
	if err := clonePlugin(ctx, url, tmp); err != nil {
		return Plugin{}, err
	}
	var man PluginManifest
	if err := readJSON(filepath.Join(tmp, "plugin.json"), &man); err != nil {
		return Plugin{}, errMsg(M("plugin.no_manifest", "url", url))
	}
	if !rePluginName.MatchString(man.Name) || strings.Contains(man.Name, "..") {
		return Plugin{}, errMsg(M("plugin.bad_name", "name", man.Name))
	}
	commit := gitx.Output(tmp, "rev-parse", "HEAD")
	if err := os.RemoveAll(filepath.Join(tmp, ".git")); err != nil {
		return Plugin{}, err
	}
	dir := filepath.Join(pluginsDir(root), man.Name)
	if exists(dir) {
		return Plugin{}, errMsg(M("plugin.exists", "name", man.Name))
	}
	if err := writeJSON(pluginOrigin(tmp), map[string]string{"source": url, "commit": commit}); err != nil {
		return Plugin{}, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		return Plugin{}, err
	}
	return loadPlugin(root, man.Name)
}

func clonePlugin(ctx context.Context, url, dir string) error {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	cmd := gitx.Command(cctx, filepath.Dir(dir), "-c", "core.hooksPath=/dev/null", "-c", "protocol.ext.allow=never", "clone", "--depth", "1", "--no-recurse-submodules", "--", url, dir)
	noConsoleWindow(cmd)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return errMsg(M("plugin.clone_failed", "error", truncate(strings.TrimSpace(string(out)), 400)))
	}
	return nil
}

// RemovePlugin deletes a package and its approval.
func RemovePlugin(root, name string) error {
	if !rePluginName.MatchString(name) {
		return errMsg(M("plugin.bad_name", "name", name))
	}
	dir := filepath.Join(pluginsDir(root), name)
	if !exists(dir) {
		return errMsg(M("plugin.not_installed", "name", name))
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return setPluginApproval(root, name, "")
}

// ListPlugins lists the installed packages.
func ListPlugins(root string) []Plugin {
	out := []Plugin{}
	entries, _ := os.ReadDir(pluginsDir(root))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if p, err := loadPlugin(root, e.Name()); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func loadPlugin(root, name string) (Plugin, error) {
	dir := filepath.Join(pluginsDir(root), name)
	var p Plugin
	if err := readJSON(filepath.Join(dir, "plugin.json"), &p.PluginManifest); err != nil {
		return p, err
	}
	var origin struct{ Source, Commit string }
	_ = readJSON(pluginOrigin(dir), &origin)
	p.Name, p.Dir, p.Source, p.Commit = name, dir, origin.Source, origin.Commit
	p.Approved = PluginApproved(root, name)
	p.Runs = pluginRuns(root, name)
	return p, nil
}

// pluginRuns is what the plugin would execute, for the approval question.
func pluginRuns(root, name string) []string {
	var runs []string
	for _, h := range pluginHooks(root, name) {
		runs = append(runs, h.Event+": "+h.Command)
	}
	servers := mcp.LoadFile(mcp.PluginFile(root, name)).Servers
	for n, c := range servers {
		runs = append(runs, "MCP "+n+": "+c.Describe())
	}
	return runs
}

// ---------------------------------------------------------------- approval

var (
	pluginApproveMu sync.Mutex
	pluginHashes    sync.Map // dir → pluginStamp
)

type pluginStamp struct{ stamp, hash string }

func pluginApprovalsFile() string { return filepath.Join(InfuseHome(), "approved-plugins.json") }

func pluginKey(root, name string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(root) + "\x00" + name))
	return hex.EncodeToString(sum[:])
}

// pluginHash digests a plugin's tree. Sizes and times are walked every time; the bytes only when they
// moved, since hooks ask this on every tool call.
func pluginHash(dir string) (string, error) {
	var names []string
	var stamp strings.Builder
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		names = append(names, p)
		fmt.Fprintf(&stamp, "%s|%d|%d\n", p, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	if c, ok := pluginHashes.Load(dir); ok && c.(pluginStamp).stamp == stamp.String() {
		return c.(pluginStamp).hash, nil
	}
	h := sha256.New()
	for _, p := range names {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	hash := hex.EncodeToString(h.Sum(nil))
	pluginHashes.Store(dir, pluginStamp{stamp.String(), hash})
	return hash, nil
}

func setPluginApproval(root, name, hash string) error {
	pluginApproveMu.Lock()
	defer pluginApproveMu.Unlock()
	m := map[string]string{}
	_ = config.OpenJSON(pluginApprovalsFile(), &m)
	if hash == "" {
		delete(m, pluginKey(root, name))
	} else {
		m[pluginKey(root, name)] = hash
	}
	return config.SealJSON(pluginApprovalsFile(), m)
}

// ApprovePlugin records the user's approval of the plugin as it stands now.
func ApprovePlugin(root, name string) error {
	dir := filepath.Join(pluginsDir(root), name)
	if !rePluginName.MatchString(name) || !exists(dir) {
		return errMsg(M("plugin.not_installed", "name", name))
	}
	hash, err := pluginHash(dir)
	if err != nil {
		return err
	}
	if err := setPluginApproval(root, name, hash); err != nil {
		return err
	}
	for server, c := range mcp.LoadFile(mcp.PluginFile(root, name)).Servers {
		if err := mcp.Approve(root, mcp.PluginScope+name, server, c); err != nil {
			return err
		}
	}
	return nil
}

// PluginApproved reports whether a signed approval covers the plugin's current content.
func PluginApproved(root, name string) bool {
	hash, err := pluginHash(filepath.Join(pluginsDir(root), name))
	if err != nil {
		return false
	}
	pluginApproveMu.Lock()
	defer pluginApproveMu.Unlock()
	m := map[string]string{}
	if config.OpenJSON(pluginApprovalsFile(), &m) != nil {
		return false // an unsigned or edited file approves nothing
	}
	return m[pluginKey(root, name)] == hash
}

// pluginRootVar in a plugin's hook command is the plugin's folder.
const pluginRootVar = "${INFUSE_PLUGIN_ROOT}"

// pluginHooks lists a plugin's hooks; they are approved with the plugin, never one by one.
func pluginHooks(root, name string) []HookEntry {
	dir := filepath.Join(pluginsDir(root), name)
	src := filepath.Join(dir, "hooks.json")
	var f hooksFile
	if readJSON(src, &f) != nil {
		return nil
	}
	ok := PluginApproved(root, name)
	var out []HookEntry
	for _, ev := range hookEvents {
		for _, g := range f.Hooks[ev] {
			for _, h := range g.Hooks {
				if h.Command == "" || (h.Type != "" && h.Type != "command") {
					continue
				}
				scope := mcp.PluginScope + name
				command := strings.ReplaceAll(h.Command, pluginRootVar, filepath.ToSlash(dir))
				out = append(out, HookEntry{Source: src, Scope: scope, Event: ev, Matcher: g.Matcher, Command: command, Timeout: h.Timeout, Approved: ok, Key: hookKey(root, scope, ev, g.Matcher, command)})
			}
		}
	}
	return out
}

func allPluginHooks(root string) []HookEntry {
	var out []HookEntry
	entries, _ := os.ReadDir(pluginsDir(root))
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, pluginHooks(root, e.Name())...)
		}
	}
	return out
}

func init() {
	mcp.PluginGate = PluginApproved
}

// ---------------------------------------------------------------- marketplace

// MarketEntry is one row of a marketplace index.
type MarketEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	URL         string `json:"url"`
	Version     string `json:"version,omitempty"`
}

// ParseMarketIndex reads an index: a list of entries, or an object with a "plugins" list.
func ParseMarketIndex(b []byte) ([]MarketEntry, error) {
	var list []MarketEntry
	if json.Unmarshal(b, &list) == nil {
		return list, nil
	}
	var doc struct {
		Plugins []MarketEntry `json:"plugins"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, errors.New("the marketplace index is not valid JSON")
	}
	return doc.Plugins, nil
}
