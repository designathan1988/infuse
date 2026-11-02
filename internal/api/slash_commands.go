package api

// Slash commands for the window: the built-ins it handles and the user's custom ones.

import (
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/data"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type slashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Group       string `json:"group,omitempty"` // session | model | view | tools | help | custom
	Locus       string `json:"locus,omitempty"` // client | server | tool | custom
	Args        string `json:"args,omitempty"`
	Prompt      string `json:"prompt,omitempty"` // custom: the file body, with $ARGUMENTS
	Builtin     bool   `json:"builtin"`
}

// listCommands: built-ins the window handles, plus custom ones from <project>/.infuse/commands/*.md and
// ~/.infuse/commands/*.md (also read from .claude/commands for compatibility).
func listCommands(root string) []slashCommand {
	out := builtinCommands()
	seen := map[string]bool{}
	dirs := []string{filepath.Join(root, core.InfuseDirName, "commands"), filepath.Join(root, ".claude", "commands")}
	dirs = append(dirs, knowledge.PluginContentDirs(root, "commands")...)
	dirs = append(dirs, filepath.Join(core.InfuseHome(), "commands"), filepath.Join(core.HomeDir(), ".claude", "commands"))
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			if seen[name] {
				continue
			}
			seen[name] = true
			body := agent.ReadIfExists(filepath.Join(dir, e.Name()))
			desc := ""
			if strings.HasPrefix(body, "---") { // frontmatter: description
				if end := strings.Index(body[3:], "---"); end > 0 {
					for _, l := range strings.Split(body[3:3+end], "\n") {
						if k, v, ok := strings.Cut(l, ":"); ok && strings.TrimSpace(k) == "description" {
							desc = strings.TrimSpace(v)
						}
					}
					body = strings.TrimSpace(body[3+end+3:])
				}
			}
			if desc == "" {
				desc = core.Truncate(strings.SplitN(strings.TrimSpace(body), "\n", 2)[0], 80)
			}
			out = append(out, slashCommand{Name: name, Description: desc, Prompt: body, Group: "custom", Locus: "custom"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Builtin && !out[j].Builtin })
	return out
}

// builtinCommands is the declarative table of commands the window handles (internal/data/commands.json).
func builtinCommands() []slashCommand {
	var list []slashCommand
	_ = json.Unmarshal(data.Read("commands.json"), &list)
	for i := range list {
		list[i].Builtin = true
	}
	return list
}
