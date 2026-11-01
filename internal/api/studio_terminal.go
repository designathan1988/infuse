package api

// Integrated terminal and slash commands for the window.

import (
	"context"
	"encoding/json"
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/data"
	"infuse/internal/filter"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type terminalResult struct {
	Output string   `json:"output"`
	Cwd    string   `json:"cwd"`
	Exit   int      `json:"exit"`
	Notes  []string `json:"notes"`
}

// runTerminal runs one command through the filter (heredoc transform, rules, journal) and returns what the
// agent would have seen, plus the new working directory.
func runTerminal(root, cwd, command string) terminalResult {
	if cwd == "" || !core.Within(root, cwd) {
		cwd = root
	}
	if strings.TrimSpace(command) == "" {
		return terminalResult{Cwd: cwd, Notes: []string{}}
	}
	rules := filter.LoadRules(root)
	tr := filter.TransformCommand(command, cwd)
	if rules.Disabled["heredoc"] {
		tr = filter.TransformResult{Command: command}
	}
	final, ruleLines := filter.ApplyCommandRules(root, rules, tr.Command)
	journal.Journal(root, journal.JournalEntry{Kind: "command", Command: command, Detail: "terminal"})
	for _, a := range tr.Actions {
		if a.Kind ==
			filter.ActLesson &&
			a.Vice != "" {
			journal.Lesson(root, a.Vice, a.Detail)
		} else {
			journal.Journal(root, journal.JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
	started := time.Now().UnixMilli()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, core.FindRealBash(), "-c", final+"\nprintf '\\n__INFUSE_CWD__%s' \"$PWD\"")
	process.NoConsoleWindow(cmd)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	buf := core.NewCapWriter(1 << 20)
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Run()
	out := buf.String()
	res := terminalResult{Cwd: cwd, Exit: core.ExitCode(err), Notes: []string{}}
	if i := strings.LastIndex(out, "__INFUSE_CWD__"); i >= 0 {
		if nc := core.FromMsysPath(strings.TrimSpace(out[i+len("__INFUSE_CWD__"):])); nc != "" &&
			core.Within(root, nc) {
			res.Cwd = nc
		}
		out = strings.TrimRight(out[:i], "\n")
	}
	res.Output = out
	if trailer := filter.BuildTrailer(root, cwd, command, tr.Actions, started, rules, ruleLines); trailer != "" {
		res.Notes = strings.Split(strings.TrimSpace(trailer), "\n")
	}
	return res
}

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
	for _, dir := range []string{
		filepath.Join(root, core.InfuseDirName, "commands"), filepath.Join(root, ".claude", "commands"),
		filepath.Join(core.InfuseHome(), "commands"), filepath.Join(core.HomeDir(), ".claude", "commands"),
	} {
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
