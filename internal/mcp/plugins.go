package mcp

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
)

// PluginScope prefixes the scope of a server that came with a plugin: "plugin:<name>".
const PluginScope = "plugin:"

// PluginGate reports whether the user approved a plugin. The agent package owns plugins and sets it;
// unset, no plugin server runs.
var PluginGate func(root, plugin string) bool

// PluginFile is the MCP file of an installed plugin.
func PluginFile(root, plugin string) string {
	return filepath.Join(root, core.InfuseDirName, "plugins", plugin, ".mcp.json")
}

// pluginEntries lists the servers of approved plugins. A server of a plugin not approved is not listed
// at all: it can neither run nor be approved on its own.
func (m *Manager) pluginEntries() []Entry {
	if PluginGate == nil {
		return nil
	}
	dirs, _ := os.ReadDir(filepath.Join(m.root, core.InfuseDirName, "plugins"))
	var out []Entry
	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") || !PluginGate(m.root, d.Name()) {
			continue
		}
		for name, c := range LoadFile(PluginFile(m.root, d.Name())).Servers {
			out = append(out, Entry{PluginScope + d.Name(), name, c})
		}
	}
	return out
}
