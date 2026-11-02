package api

// Slash commands for the window: the built-ins it handles and the user's custom ones.

import (
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type slashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt,omitempty"` // custom: the file body, with $ARGUMENTS
	Builtin     bool   `json:"builtin"`
}

// listCommands: built-ins the window handles, plus custom ones from <project>/.infuse/commands/*.md and
// ~/.infuse/commands/*.md (also read from .claude/commands for compatibility).
func listCommands(root string) []slashCommand {
	out := []slashCommand{
		{Name: "clear", Description: "slash.clear", Builtin: true},
		{Name: "compact", Description: "slash.compact", Builtin: true},
		{Name: "model", Description: "slash.model", Builtin: true},
		{Name: "mode", Description: "slash.mode", Builtin: true},
		{Name: "rewind", Description: "slash.rewind", Builtin: true},
		{Name: "diff", Description: "slash.diff", Builtin: true},
		{Name: "cost", Description: "slash.cost", Builtin: true},
		{Name: "vim", Description: "slash.vim", Builtin: true},
		{Name: "copy", Description: "slash.copy", Builtin: true},
		{Name: "export", Description: "slash.export", Builtin: true},
		{Name: "btw", Description: "slash.btw", Builtin: true},
		{Name: "mission", Description: "slash.mission", Builtin: true},
		{Name: "plan", Description: "slash.plan", Builtin: true},
		{Name: "effort", Description: "slash.effort", Builtin: true},
		{Name: "config", Description: "slash.config", Builtin: true},
		{Name: "resume", Description: "slash.resume", Builtin: true},
		{Name: "rename", Description: "slash.rename", Builtin: true},
		{Name: "review", Description: "slash.review", Builtin: true},
		{Name: "init", Description: "slash.initDesc", Builtin: true},
		{Name: "memory", Description: "slash.memory", Builtin: true},
		{Name: "terminal", Description: "slash.terminal", Builtin: true},
		{Name: "help", Description: "slash.help", Builtin: true},
	}
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
			out = append(out, slashCommand{Name: name, Description: desc, Prompt: body})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Builtin && !out[j].Builtin })
	return out
}
