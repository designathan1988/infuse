package main

// The guard: protected paths (.infuse/oracle/** always, plus the user's list) are unreachable for the
// agent through EVERY tool, enforced in the executor, never by text. Write/Edit are refused at the path
// gate (resolvePath); shell commands pass through GuardCommand before anything runs or is transformed:
//   - a heredoc whose target is protected → the whole command is refused (checked before the transform
//     writes the file);
//   - a redirection (>, >>, >|) into a protected path → neutralized to /dev/null, the model is told;
//   - a destructive or file-writing command (rm, mv, cp, tee, sed -i, truncate, dd of=, install, ln)
//     naming a protected path → refused.
// Every attempt goes to the journal as "blocked" and to the lessons.

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const oracleRel = ".infuse/oracle"

type Guard struct {
	Root      string
	Protected []string // globs relative to root
}

func newGuard(root string, protected []string) *Guard {
	return &Guard{Root: root, Protected: append([]string{oracleRel + "/**", oracleRel}, protected...)}
}

// protects reports whether abs (or a path relative to cwd) is agent-proof.
func (g *Guard) protects(cwd, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || p == "/dev/null" {
		return "", false
	}
	p = FromMsysPath(p)
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(cwd, p)
	}
	rel := relTo(g.Root, filepath.Clean(abs))
	// the oracle is protected wherever it is: the project's, a worktree's, or reached from outside the root
	if slash := "/" + strings.Trim(filepath.ToSlash(filepath.Clean(abs)), "/"); strings.Contains(slash+"/", "/"+oracleRel+"/") {
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = filepath.ToSlash(filepath.Clean(abs))
		}
		return rel, true
	}
	if rel == "" || strings.HasPrefix(rel, "..") {
		return rel, false
	}
	for _, pat := range g.Protected {
		if globToRegex(pat).MatchString(rel) || rel == strings.TrimSuffix(pat, "/**") {
			return rel, true
		}
	}
	return rel, false
}

func (g *Guard) block(rel, detail string) error {
	Journal(g.Root, JournalEntry{Kind: "blocked", Path: rel, Detail: detail})
	Lesson(g.Root, "protected-path-write", detail+": "+rel)
	return fmt.Errorf("caminho protegido, somente leitura para o agente: %s (%s). O oráculo e os caminhos protegidos são definidos pelo usuário e não podem ser alterados pelo agente", rel, detail)
}

var reRedir = regexp.MustCompile(`(?:^|[^<>&0-9])(\d?)(>>|>\||>)\s*("[^"]*"|'[^']*'|[^\s;&|<>()]+)`)

// writingCommands: commands whose non-flag arguments are paths they create, move or delete.
var writingCommands = map[string]bool{"rm": true, "rmdir": true, "mv": true, "cp": true, "tee": true, "truncate": true, "install": true, "ln": true, "shred": true, "unlink": true, "rsync": true, "chmod": true, "chown": true, "touch": true, "mkdir": true}

// GuardCommand inspects a shell command before it runs. It returns the (possibly rewritten) command,
// notes for the model, or an error when the command must not run at all.
func (g *Guard) GuardCommand(cmd, cwd string) (string, []string, error) {
	// 1. heredocs aimed at protected files: refuse before the transform can write them
	for _, h := range FindHeredocs(cmd) {
		_, _, line := CommandLineOf(cmd, h)
		if intent := DetectFileWrite(line); intent != nil {
			if rel, ok := g.protects(cwd, intent.Path); ok {
				return "", nil, g.block(rel, "heredoc para caminho protegido recusado")
			}
		}
	}
	var notes []string
	// 2. destructive / writing commands naming a protected path: refuse
	for _, seg := range splitShell(cmd) {
		fields := shellFields(seg)
		if len(fields) == 0 {
			continue
		}
		head := filepath.Base(strings.Trim(fields[0], `"'`))
		if head == "sudo" || head == "env" || head == "command" || head == "exec" {
			if len(fields) > 1 {
				fields = fields[1:]
				head = filepath.Base(strings.Trim(fields[0], `"'`))
			}
		}
		check := writingCommands[head] || (head == "sed" && hasFlag(fields, "-i")) || head == "dd" || head == "git" && len(fields) > 1 && (fields[1] == "clean" || fields[1] == "checkout" || fields[1] == "restore") || head == "python" || head == "python3" || head == "node" || head == "perl" || head == "pwsh" || head == "powershell"
		if !check {
			continue
		}
		for _, f := range fields[1:] {
			arg := strings.TrimPrefix(f, "of=")
			if strings.HasPrefix(arg, "-") && !strings.Contains(arg, "/") {
				continue
			}
			for _, candidate := range strings.Split(arg, "=") { // --target=path, of=path
				if rel, ok := g.protects(cwd, candidate); ok {
					return "", nil, g.block(rel, "comando sobre caminho protegido recusado: "+truncate(strings.TrimSpace(seg), 120))
				}
			}
		}
	}
	// 3. redirections into protected paths: /dev/null
	out := reRedir.ReplaceAllStringFunc(cmd, func(m string) string {
		sm := reRedir.FindStringSubmatch(m)
		if rel, ok := g.protects(cwd, sm[3]); ok {
			notes = append(notes, "[infuse] redirecionamento para caminho protegido neutralizado: "+rel)
			_ = g.block(rel, "redirecionamento neutralizado")
			return strings.Replace(m, sm[3], "/dev/null", 1)
		}
		return m
	})
	return out, notes, nil
}

func hasFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag || strings.HasPrefix(f, flag) && !strings.HasPrefix(f, "--") {
			return true
		}
	}
	return false
}

// splitShell splits a command at top-level ; && || | and newlines, respecting quotes and parentheses.
func splitShell(cmd string) []string {
	var segs []string
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
		case c == '(' || c == '{' || c == '`':
			depth++
		case c == ')' || c == '}':
			depth--
		case depth <= 0 && (c == ';' || c == '\n' || c == '|' || c == '&'):
			segs = append(segs, cmd[last:i])
			for i+1 < len(cmd) && (cmd[i+1] == '&' || cmd[i+1] == '|') {
				i++
			}
			last = i + 1
		}
	}
	segs = append(segs, cmd[last:])
	return segs
}

// shellFields splits one simple command into words, keeping quoted strings together (quotes stripped).
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
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inS, has = true, true
		case c == '"':
			inD, has = true, true
		case c == ' ' || c == '\t':
			flush()
		case c == '>' || c == '<':
			flush() // redirections are handled separately
			for i+1 < len(seg) && (seg[i+1] == '>' || seg[i+1] == '|' || seg[i+1] == ' ') {
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
