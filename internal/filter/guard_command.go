package filter

import (
	"infuse/internal/core"
	"infuse/internal/journal"
	"path/filepath"
	"regexp"
	"strings"
)

// Command policy: redirections, writing commands and nested shells.

var ReRedir = regexp.MustCompile(`(?:^|[^<>&0-9])(\d*)(&>>|&>|>>|>\||<>|>&|>)\s*("[^"]*"|'[^']*'|[^\s;&|<>()]+)`)

// writingCommands: commands whose path arguments are created, moved, changed or deleted.
var writingCommands = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true, "tee": true, "truncate": true, "install": true, "ln": true, "shred": true,
	"unlink": true, "rsync": true, "chmod": true, "chown": true, "touch": true, "mkdir": true, "dd": true, "patch": true,
	"mklink": true, "junction": true, // a link is a way to reach a protected tree under another name
	// cmd.exe and PowerShell, reached through cmd /c or powershell -Command
	"del": true, "erase": true, "rd": true, "move": true, "copy": true, "ren": true, "rename": true, "xcopy": true, "robocopy": true,
	"remove-item": true, "ri": true, "move-item": true, "mi": true, "copy-item": true, "cpi": true, "set-content": true,
	"add-content": true, "ac": true, "out-file": true, "new-item": true, "ni": true, "clear-content": true, "clc": true, "rename-item": true, "rni": true,
}

// scripting commands can write anything their arguments name; a protected path among them is refused.
var scriptingCommands = map[string]bool{"python": true, "python3": true, "node": true, "perl": true, "ruby": true, "php": true, "deno": true, "bun": true}

type shellSeg struct {
	text       string
	start, end int    // offsets in the command
	sep        string // operator after this segment: ; && || | & newline ( ) {, or ""
}

type guardEdit struct {
	start, end int
	with       string
}

// guardRun is one command under inspection.
type guardRun struct {
	cmd    string
	hds    []Heredoc
	segs   []shellSeg
	nested bool
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

// Reguard judges a command a rule rewrote after the guard saw it: what finally runs is what is guarded.
func (g *Guard) Reguard(before, after, cwd string) (string, []core.Msg, error) {
	if before == after {
		return after, nil, nil
	}
	return g.GuardCommandMsgs(after, cwd)
}

func (g *Guard) guard(cmd, cwd string, nested bool) (string, []core.Msg, error) {
	hds := FindHeredocs(cmd)
	run := guardRun{cmd: cmd, hds: hds, segs: splitSegments(maskHeredocBodies(cmd, hds)), nested: nested}
	var notes []core.Msg
	var edits []guardEdit
	walk := newDirWalk(cwd)
	pipeStart := 0
	for i, sg := range run.segs {
		ns, es, err := g.checkSegment(run, i, pipeStart, walk.dir)
		if err != nil {
			return "", nil, err
		}
		notes = append(notes, ns...)
		edits = append(edits, es...)
		walk.step(sg) // the directory for the next segment
		if sg.sep != "|" && sg.sep != "(" && sg.sep != ")" {
			pipeStart = i + 1
		}
	}
	return applyGuardEdits(cmd, edits), notes, nil
}

// checkSegment judges one simple command with the directory in effect at it.
func (g *Guard) checkSegment(run guardRun, i, pipeStart int, dir string) ([]core.Msg, []guardEdit, error) {
	sg := run.segs[i]
	if err := g.checkHeredocTargets(sg, run, dir); err != nil {
		return nil, nil, err
	}
	for _, script := range substitutions(sg.text) {
		if _, _, err := g.guard(script, dir, true); err != nil {
			return nil, nil, err
		}
	}
	if err := g.checkInputRedirects(sg.text, dir); err != nil {
		return nil, nil, err
	}
	if fields := commandWords(shellFields(sg.text)); len(fields) > 0 {
		if err := g.checkSimple(fields, dir, sg.text); err != nil {
			return nil, nil, err
		}
		// xargs running a writer or a reader: everything the pipeline names is a potential target
		if err := g.checkXargs(fields, run.segs[pipeStart:i+1], dir); err != nil {
			return nil, nil, err
		}
	}
	return g.redirEdits(sg, dir, run.nested)
}

// maskHeredocBodies blanks heredoc bodies (same length, newlines kept): offsets stay valid and
// document text is inert.
func maskHeredocBodies(cmd string, hds []Heredoc) string {
	masked := []byte(cmd)
	for _, h := range hds {
		for i := h.BodyStart; i < h.BlockEnd && i < len(masked); i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}
	return string(masked)
}

// checkHeredocTargets refuses a heredoc whose command writes a protected path, or a path the agent's
// Write could not reach, with the directory in effect at that segment. The transform runs on the host,
// outside the OS sandbox, so this is the only wall such a write has. A nested shell's heredoc is not
// transformed: it runs inside the sandbox.
func (g *Guard) checkHeredocTargets(sg shellSeg, run guardRun, dir string) error {
	for _, h := range run.hds {
		if h.OpStart < sg.start || h.OpStart >= sg.end {
			continue
		}
		intent := DetectFileWrite(strings.TrimSpace(run.cmd[sg.start:h.OpEnd]))
		if intent == nil {
			continue
		}
		if rel, ok := g.Protects(dir, intent.Path); ok {
			return g.block(rel, core.M("guard.heredoc"))
		}
		if !run.nested && g.outsideWork(dir, intent.Path) {
			return g.blockOutside(filepath.ToSlash(CanonPath(dir, intent.Path)))
		}
	}
	return nil
}

// blockOutside records a heredoc refused for where it lands: the path is ordinary, the place is not.
func (g *Guard) blockOutside(p string) error {
	m := core.M("guard.heredoc_outside", "path", p)
	journal.Journal(g.Root, journal.JournalEntry{Kind: "blocked", Path: p, Detail: m.String()})
	return core.ErrMsg(m)
}

// outsideWork reports whether a path lands outside the folder the agent writes in: the session's
// worktree, or the project. A path the shell must expand is decided at run time, inside the sandbox.
func (g *Guard) outsideWork(dir, p string) bool {
	if isNullDevice(p) || strings.ContainsAny(p, "$`") {
		return false
	}
	boundary := g.Root
	if g.Work != "" {
		boundary = g.Work
	}
	return !core.Within(RealPath(boundary), RealPath(CanonPath(dir, p)))
}

func isNullDevice(p string) bool { return p == "/dev/null" || strings.EqualFold(p, "NUL") }

// checkXargs refuses a pipeline in which xargs runs a writer or a reader and the pipeline names a path
// that command may not touch.
func (g *Guard) checkXargs(fields []string, pipeline []shellSeg, dir string) error {
	if baseCmd(fields[0]) != "xargs" {
		return nil
	}
	writes, reads := xargsWrites(fields), xargsReads(fields)
	if !writes && !reads {
		return nil
	}
	for _, p := range pipeline {
		for _, f := range shellFields(p.text) {
			if rel, ok := g.Protects(dir, f); ok && writes {
				return g.block(rel, core.M("guard.xargs"))
			}
			if rel, ok := g.readsHidden(dir, f); ok {
				return g.blockRead(rel)
			}
		}
	}
	return nil
}

// redirEdits neutralizes redirections into protected paths; inside a nested shell it refuses instead.
func (g *Guard) redirEdits(sg shellSeg, dir string, nested bool) ([]core.Msg, []guardEdit, error) {
	var notes []core.Msg
	var edits []guardEdit
	for _, loc := range redirTargets(sg.text) {
		target := sg.text[loc[0]:loc[1]]
		if rel, ok := g.Protects(dir, target); ok {
			if nested {
				return nil, nil, g.block(rel, core.M("guard.nested_redir"))
			}
			notes = append(notes, core.M("guard.redirected", "file", rel))
			_ = g.block(rel, core.M("guard.redir_blocked"))
			edits = append(edits, guardEdit{sg.start + loc[0], sg.start + loc[1], "/dev/null"})
		}
	}
	return notes, edits, nil
}

// redirTargets gives the offsets of every redirection target. Each search restarts right after the
// previous target, so `>a>b` is two redirections although the first one ate the second's left side.
func redirTargets(text string) [][2]int {
	var out [][2]int
	for pos := 0; pos < len(text); {
		loc := ReRedir.FindStringSubmatchIndex(text[pos:])
		if loc == nil {
			break
		}
		out = append(out, [2]int{pos + loc[6], pos + loc[7]})
		pos += loc[7]
	}
	return out
}

// applyGuardEdits rewrites the command, last edit first so earlier offsets stay valid.
func applyGuardEdits(cmd string, edits []guardEdit) string {
	out := cmd
	for i := len(edits) - 1; i >= 0; i-- {
		out = out[:edits[i].start] + edits[i].with + out[edits[i].end:]
	}
	return out
}

func baseCmd(f string) string {
	return strings.TrimSuffix(strings.ToLower(filepath.Base(strings.ReplaceAll(strings.Trim(f, `"'`), `\`, "/"))), ".exe")
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
			if psFlag(a, "command") && i+1 < len(args) {
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

// xargsTool is the command xargs runs ("" when it runs the default echo).
func xargsTool(fields []string) string {
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") {
			return baseCmd(f)
		}
	}
	return ""
}

func xargsWrites(fields []string) bool {
	tool := xargsTool(fields)
	return writingCommands[tool] || scriptingCommands[tool]
}

func xargsReads(fields []string) bool { return readingCommands[xargsTool(fields)] }

func resolveDir(cur, to string) string {
	return CanonPath(cur, strings.Trim(to, `"'`))
}

func hasFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag || strings.HasPrefix(f, flag) && !strings.HasPrefix(f, "--") {
			return true
		}
	}
	return false
}
