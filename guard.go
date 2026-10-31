package main

// The guard: protected paths (.infuse/oracle/** always, plus the user's list) are unreachable for the
// agent through EVERY tool, enforced in the executor, never by text. Write/Edit are refused at the path
// gate (resolvePath); shell commands pass through GuardCommand before anything runs or is transformed.
//
// A command is read the way the shell will run it: split into simple commands (;, &&, ||, |, &, newline),
// with the working directory followed across cd / pushd / popd, pipelines kept together, the payload of
// nested shells (bash -c, sh -c, eval, cmd /c, powershell -Command) guarded strictly, and symlinks resolved
// to their targets. Heredoc bodies are masked first, so document text never looks like a command.
//   - heredoc into a protected file             → refused (before the transform could write it)
//   - redirection (>, >>, >|, <>) into one       → neutralized to /dev/null, the model is told
//     (inside a nested shell: refused — the payload cannot be rewritten safely)
//   - a writing/destroying command naming one    → refused (rm, mv, cp, tee, sed -i, find -delete/-exec,
//     xargs with a writer in a pipeline that names one, git rm/clean/stash/checkout/restore, del,
//     Remove-Item…)
// Every attempt goes to the journal as "blocked" and to the lessons.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	oracleRel   = ".infuse/oracle"
	evidenceRel = ".infuse/evidence" // the executor's verdicts and evidence: written only by the executor
)

type Guard struct {
	Root      string
	Protected []string // globs relative to root
}

// infuse's own records (other sessions' transcripts, the journal, snapshots, checkpoints) are not the
// project: the agent neither reads nor writes them, by any tool.
var internalRels = []string{".infuse/sessions", ".infuse/checkpoints", ".infuse/tree", ".infuse/journal.jsonl", ".infuse/watcher.json"}

// agentVisibleInternal lists what of .infuse/ the agent may read: pinned docs, the conventions, the
// project map and its own captures.
var agentVisibleInternal = []string{".infuse/docs/", ".infuse/conventions.md", ".infuse/map/MAP.md", ".infuse/shots/", ".infuse/recordings/", ".infuse/skills/"}

// hiddenInternal reports whether rel (relative to the project) is infuse's own file the agent may not read.
func hiddenInternal(rel string) bool {
	rel = strings.TrimPrefix(toPosix(rel), "./")
	if rel != ".infuse" && !strings.HasPrefix(rel, ".infuse/") {
		return false
	}
	for _, ok := range agentVisibleInternal {
		if rel == strings.TrimSuffix(ok, "/") || strings.HasPrefix(rel, ok) {
			return false
		}
	}
	return true
}

func newGuard(root string, protected []string) *Guard {
	base := []string{oracleRel + "/**", oracleRel, evidenceRel + "/**", evidenceRel, memoryRel + "/**", memoryRel}
	for _, r := range internalRels {
		base = append(base, r+"/**", r)
	}
	return &Guard{Root: root, Protected: append(base, protected...)}
}

// protects reports whether p (absolute, or relative to cwd) is agent-proof. A symlink counts as its target.
func (g *Guard) protects(cwd, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || p == "/dev/null" || strings.EqualFold(p, "NUL") || strings.HasPrefix(p, "-") {
		return "", false
	}
	p = FromMsysPath(strings.ReplaceAll(p, `\`, "/"))
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(cwd, p)
	}
	abs = filepath.Clean(abs)
	if rel, ok := g.protectsAbs(abs); ok {
		return rel, true
	}
	if real := resolveSymlinks(abs); real != abs {
		if rel, ok := g.protectsAbs(real); ok {
			return rel, true
		}
	}
	return "", false
}

func (g *Guard) protectsAbs(abs string) (string, bool) {
	rel := relTo(g.Root, abs)
	// the oracle is protected wherever it is: the project's, a worktree's, or reached from outside the root
	if slash := "/" + strings.Trim(filepath.ToSlash(abs), "/"); strings.Contains(slash+"/", "/"+oracleRel+"/") || strings.Contains(slash+"/", "/"+evidenceRel+"/") {
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = filepath.ToSlash(abs)
		}
		return rel, true
	}
	if rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}
	for _, pat := range g.Protected {
		if globToRegex(pat).MatchString(rel) || rel == strings.TrimSuffix(pat, "/**") {
			return rel, true
		}
	}
	return "", false
}

// resolveSymlinks resolves a path that may not exist yet (its deepest existing parent is resolved).
func resolveSymlinks(abs string) string {
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(r, filepath.Base(abs))
	}
	return abs
}

func (g *Guard) block(rel, detail string) error {
	Journal(g.Root, JournalEntry{Kind: "blocked", Path: rel, Detail: detail})
	Lesson(g.Root, "protected-path-write", detail+": "+rel)
	return fmt.Errorf("caminho protegido, somente leitura para o agente: %s (%s). O oráculo e os caminhos protegidos são definidos pelo usuário e não podem ser alterados pelo agente", rel, detail)
}

// reRedir: output redirections and the read-write open `<>` (group 3 is the target).
var reRedir = regexp.MustCompile(`(?:^|[^<>&0-9])(\d?)(>>|>\||<>|>)\s*("[^"]*"|'[^']*'|[^\s;&|<>()]+)`)

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
// notes for the model, or an error when the command must not run at all.
func (g *Guard) GuardCommand(cmd, cwd string) (string, []string, error) {
	return g.guard(cmd, cwd, false)
}

func (g *Guard) guard(cmd, cwd string, nested bool) (string, []string, error) {
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
	var notes []string
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
					if rel, ok := g.protects(dir, intent.Path); ok {
						return "", nil, g.block(rel, "heredoc para caminho protegido recusado")
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
						if rel, ok := g.protects(dir, f); ok {
							return "", nil, g.block(rel, "xargs com comando destrutivo sobre caminho protegido recusado")
						}
					}
				}
			}
		}
		// redirections into protected paths
		for _, loc := range reRedir.FindAllStringSubmatchIndex(sg.text, -1) {
			target := sg.text[loc[6]:loc[7]]
			if rel, ok := g.protects(dir, target); ok {
				if nested {
					return "", nil, g.block(rel, "redirecionamento em shell aninhado para caminho protegido recusado")
				}
				notes = append(notes, "[infuse] redirecionamento para caminho protegido neutralizado: "+rel)
				_ = g.block(rel, "redirecionamento neutralizado")
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
	refuse := func(why string) error {
		for _, a := range args {
			for _, cand := range strings.Split(strings.TrimPrefix(a, "of="), "=") {
				if rel, ok := g.protects(dir, cand); ok {
					return g.block(rel, why+": "+truncate(strings.TrimSpace(text), 120))
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
		return refuse("comando sobre caminho protegido recusado")
	case head == "sed" && hasFlag(args, "-i"):
		return refuse("sed -i sobre caminho protegido recusado")
	case head == "find" && findWrites(args):
		return refuse("find que altera caminho protegido recusado")
	case head == "git" && len(args) > 0 && gitWriting[args[0]]:
		return refuse("git " + args[0] + " sobre caminho protegido recusado")
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
	to = FromMsysPath(strings.ReplaceAll(strings.Trim(to, `"'`), `\`, "/"))
	if strings.HasPrefix(to, "~") {
		to = filepath.Join(homeDir(), to[1:])
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
func splitSegments(cmd string) []shellSeg {
	var segs []shellSeg
	depth, last := 0, 0
	inS, inD := false, false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case inS:
			if c == '\'' {
				inS = false
			}
		case inD:
			if c == '"' {
				inD = false
			} else if c == '\\' {
				i++
			}
		case c == '\\':
			i++
		case c == '\'':
			inS = true
		case c == '"':
			inD = true
		case c == '(' || c == '{':
			depth++
		case c == ')' || c == '}':
			depth--
		case depth <= 0 && (c == ';' || c == '\n' || c == '|' || c == '&'):
			if c == '&' && (i > 0 && (cmd[i-1] == '>' || cmd[i-1] == '<') || i+1 < len(cmd) && cmd[i+1] == '>') {
				continue // 2>&1, &>file: redirections, not separators
			}
			if c == '|' && i > 0 && cmd[i-1] == '>' {
				continue // >| file
			}
			end, sep := i, string(c)
			if i+1 < len(cmd) && cmd[i+1] == c && (c == '&' || c == '|') {
				sep += string(c)
				i++
			}
			segs = append(segs, shellSeg{text: cmd[last:end], start: last, end: end, sep: sep})
			last = i + 1
		}
	}
	return append(segs, shellSeg{text: cmd[last:], start: last, end: len(cmd)})
}

// splitShell is the text-only form of splitSegments.
func splitShell(cmd string) []string {
	var out []string
	for _, s := range splitSegments(cmd) {
		out = append(out, s.text)
	}
	return out
}

// shellFields splits one simple command into words, keeping quoted strings together (quotes stripped)
// and leaving redirections out (they are handled separately).
func shellFields(seg string) []string {
	var out []string
	var cur strings.Builder
	inS, inD, has := false, false, false
	flush := func() {
		if has {
			out = append(out, cur.String())
			cur.Reset()
			has = false
		}
	}
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch {
		case inS:
			if c == '\'' {
				inS = false
			} else {
				cur.WriteByte(c)
			}
		case inD:
			if c == '"' {
				inD = false
			} else if c == '\\' && i+1 < len(seg) && strings.IndexByte(`"\$`+"`", seg[i+1]) >= 0 {
				i++
				cur.WriteByte(seg[i])
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inS, has = true, true
		case c == '"':
			inD, has = true, true
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		case c == '>' || c == '<':
			flush()
			for i+1 < len(seg) && (seg[i+1] == '>' || seg[i+1] == '<' || seg[i+1] == '|' || seg[i+1] == '&' || seg[i+1] == ' ') {
				i++
			}
			for i+1 < len(seg) && seg[i+1] != ' ' {
				i++
			}
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	flush()
	return out
}
