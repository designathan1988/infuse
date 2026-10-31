package filter

import (
	"infuse/internal/core"
	"path/filepath"
	"regexp"
	"strings"
)

// Command policy: redirections, writing commands and nested shells.

var ReRedir = regexp.MustCompile(`(?:^|[^<>&0-9])(\d?)(>>|>\||<>|>)\s*("[^"]*"|'[^']*'|[^\s;&|<>()]+)`)

// writingCommands: commands whose path arguments are created, moved, changed or deleted.
var writingCommands = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true, "tee": true, "truncate": true, "install": true, "ln": true, "shred": true,
	"unlink": true, "rsync": true, "chmod": true, "chown": true, "touch": true, "mkdir": true, "dd": true, "patch": true,
	// cmd.exe and PowerShell, reached through cmd /c or powershell -Command
	"del": true, "erase": true, "rd": true, "move": true, "copy": true, "ren": true, "rename": true, "xcopy": true, "robocopy": true,
	"remove-item": true, "ri": true, "move-item": true, "mi": true, "copy-item": true, "cpi": true, "set-content": true,
	"add-content": true, "ac": true, "out-file": true, "new-item": true, "ni": true, "clear-content": true, "clc": true, "rename-item": true, "rni": true,
}

// scripting commands can write anything their arguments name; a protected path among them is refused.
var scriptingCommands = map[string]bool{"python": true, "python3": true, "node": true, "perl": true, "ruby": true, "php": true, "deno": true, "bun": true}

var gitWriting = map[string]bool{"rm": true, "clean": true, "stash": true, "checkout": true, "restore": true, "reset": true, "mv": true, "apply": true}

type shellSeg struct {
	text       string
	start, end int    // offsets in the command
	sep        string // operator after this segment: ; && || | & newline, or ""
}

// GuardCommand inspects a shell command before it runs. It returns the (possibly rewritten) command,
// notes for the model (English), or an error when the command must not run at all (the error carries
// its message: msgOf). GuardCommandMsgs gives the notes as messages.
func (g *Guard) GuardCommand(cmd, cwd string) (string, []string, error) {
	out, notes, err := g.guard(cmd, cwd, false)
	var lines []string
	for _, n := range notes {
		lines = append(lines, n.String())
	}
	return out, lines, err
}

// GuardCommandMsgs is GuardCommand with the notes as messages, for a caller that records them with the
// call (so the user reads them in their language).
func (g *Guard) GuardCommandMsgs(cmd, cwd string) (string, []core.Msg, error) {
	return g.guard(cmd, cwd, false)
}

func (g *Guard) guard(cmd, cwd string, nested bool) (string, []core.Msg, error) {
	// mask heredoc bodies (same length, newlines kept): offsets stay valid and document text is inert
	masked := []byte(cmd)
	hds := FindHeredocs(cmd)
	for _, h := range hds {
		for i := h.BodyStart; i < h.BlockEnd && i < len(masked); i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}
	segs := splitSegments(string(masked))
	var notes []core.Msg
	type edit struct {
		start, end int
		with       string
	}
	var edits []edit
	dir := cwd
	var stack []string
	pipeStart := 0
	for i, sg := range segs {
		fields := shellFields(sg.text)
		// a heredoc whose command is this segment: its target, with the directory in effect here
		for _, h := range hds {
			if h.OpStart >= sg.start && h.OpStart < sg.end {
				if intent := DetectFileWrite(strings.TrimSpace(cmd[sg.start:h.OpEnd])); intent != nil {
					if rel, ok := g.Protects(dir, intent.Path); ok {
						return "", nil, g.block(rel, core.M("guard.heredoc"))
					}
				}
			}
		}
		if len(fields) > 0 {
			if err := g.checkSimple(fields, dir, sg.text); err != nil {
				return "", nil, err
			}
			// xargs running a writer: everything the pipeline names is a potential target
			if baseCmd(fields[0]) == "xargs" && xargsWrites(fields) {
				for _, p := range segs[pipeStart : i+1] {
					for _, f := range shellFields(p.text) {
						if rel, ok := g.Protects(dir, f); ok {
							return "", nil, g.block(rel, core.M("guard.xargs"))
						}
					}
				}
			}
		}
		// redirections into protected paths
		for _, loc := range ReRedir.FindAllStringSubmatchIndex(sg.text, -1) {
			target := sg.text[loc[6]:loc[7]]
			if rel, ok := g.Protects(dir, target); ok {
				if nested {
					return "", nil, g.block(rel, core.M("guard.nested_redir"))
				}
				notes = append(notes, core.M("guard.redirected", "file", rel))
				_ = g.block(rel, core.M("guard.redir_blocked"))
				edits = append(edits, edit{sg.start + loc[6], sg.start + loc[7], "/dev/null"})
			}
		}
		// the directory for the next segment
		if len(fields) > 0 {
			switch baseCmd(fields[0]) {
			case "cd", "chdir", "set-location", "sl":
				if t := firstArg(fields); t != "" && t != "-" {
					dir = resolveDir(dir, t)
				} else if t == "" {
					dir = g.Root
				}
			case "pushd", "push-location":
				if t := firstArg(fields); t != "" {
					stack = append(stack, dir)
					dir = resolveDir(dir, t)
				}
			case "popd", "pop-location":
				if n := len(stack); n > 0 {
					dir, stack = stack[n-1], stack[:n-1]
				}
			}
		}
		if sg.sep != "|" {
			pipeStart = i + 1
		}
	}
	out := cmd
	for i := len(edits) - 1; i >= 0; i-- {
		out = out[:edits[i].start] + edits[i].with + out[edits[i].end:]
	}
	return out, notes, nil
}

// checkSimple refuses a simple command that writes, moves or deletes a protected path.
func (g *Guard) checkSimple(fields []string, dir, text string) error {
	for len(fields) > 0 {
		switch baseCmd(fields[0]) {
		case "sudo", "env", "command", "exec", "nohup", "time", "nice", "builtin":
			fields = fields[1:]
			for len(fields) > 0 && (strings.HasPrefix(fields[0], "-") || strings.Contains(fields[0], "=")) {
				fields = fields[1:]
			}
			continue
		}
		break
	}
	if len(fields) == 0 {
		return nil
	}
	head, args := baseCmd(fields[0]), fields[1:]
	refuse := func(why core.Msg) error {
		for _, a := range args {
			for _, cand := range strings.Split(strings.TrimPrefix(a, "of="), "=") {
				if rel, ok := g.Protects(dir, cand); ok {
					return g.block(rel, core.M("guard.with_command", "why", why, "command", core.Truncate(strings.TrimSpace(text), 120)))
				}
			}
		}
		return nil
	}
	if payload := nestedPayload(head, args); payload != "" {
		_, _, err := g.guard(payload, dir, true)
		return err
	}
	switch {
	case writingCommands[head], scriptingCommands[head]:
		return refuse(core.M("guard.command"))
	case head == "sed" && hasFlag(args, "-i"):
		return refuse(core.M("guard.sed"))
	case head == "find" && findWrites(args):
		return refuse(core.M("guard.find"))
	case head == "git" && len(args) > 0 && gitWriting[args[0]]:
		return refuse(core.M("guard.git", "sub", args[0]))
	}
	return nil
}

func baseCmd(f string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(strings.ReplaceAll(strings.Trim(f, `"'`), `\`, "/"))), ".exe")
}

func firstArg(fields []string) string {
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") {
			return f
		}
	}
	return ""
}

// nestedPayload returns the script a shell command runs (bash -c "…", eval …, cmd /c …, powershell -Command …).
func nestedPayload(head string, args []string) string {
	switch head {
	case "bash", "sh", "zsh", "dash", "ksh", "fish":
		for i, a := range args {
			if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "c") && i+1 < len(args) {
				return args[i+1]
			}
		}
	case "eval":
		return strings.Join(args, " ")
	case "cmd":
		for i, a := range args {
			if (strings.EqualFold(a, "/c") || strings.EqualFold(a, "/k")) && i+1 < len(args) {
				return strings.Join(args[i+1:], " ")
			}
		}
	case "powershell", "pwsh":
		for i, a := range args {
			if (strings.EqualFold(a, "-command") || strings.EqualFold(a, "-c")) && i+1 < len(args) {
				return strings.Join(args[i+1:], " ")
			}
		}
	}
	return ""
}

func findWrites(args []string) bool {
	for i, a := range args {
		switch a {
		case "-delete", "-fprint", "-fprintf", "-fls":
			return true
		case "-exec", "-execdir", "-ok", "-okdir":
			if i+1 < len(args) && (writingCommands[baseCmd(args[i+1])] || scriptingCommands[baseCmd(args[i+1])]) {
				return true
			}
		}
	}
	return false
}

func xargsWrites(fields []string) bool {
	for _, f := range fields[1:] {
		if strings.HasPrefix(f, "-") {
			continue
		}
		return writingCommands[baseCmd(f)] || scriptingCommands[baseCmd(f)]
	}
	return false
}

func resolveDir(cur, to string) string {
	to = core.FromMsysPath(strings.ReplaceAll(strings.Trim(to, `"'`), `\`, "/"))
	if strings.HasPrefix(to, "~") {
		to = filepath.Join(core.HomeDir(), to[1:])
	}
	if filepath.IsAbs(to) {
		return filepath.Clean(to)
	}
	return filepath.Clean(filepath.Join(cur, to))
}

func hasFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag || strings.HasPrefix(f, flag) && !strings.HasPrefix(f, "--") {
			return true
		}
	}
	return false
}

// splitSegments splits a command at top-level ; && || | & and newlines, respecting quotes, escapes and
// parentheses, recording each segment's offsets and the operator after it.
