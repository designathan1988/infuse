package filter

// One simple command: what it really runs, past keywords, assignments and wrappers, and what that
// command does to the paths it names.

import (
	"infuse/internal/core"
	"regexp"
	"strings"
)

var (
	reAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*\+?=`)
	reDuration   = regexp.MustCompile(`^\d+(\.\d+)?[smhd]?$`)
)

// shellKeywords come before a command without being one.
var shellKeywords = map[string]bool{"then": true, "do": true, "else": true, "elif": true, "if": true, "while": true, "until": true, "!": true, "{": true, "}": true}

// commandWrappers run the rest of the line as a command; the value is their flags that take a value.
var commandWrappers = map[string]string{
	"sudo": "-u -g -h -p -C -r -t -T -U", "doas": "-u -C", "env": "-u -C -S", "command": "", "exec": "-a", "builtin": "",
	"nohup": "", "time": "-f -o", "nice": "-n", "ionice": "-c -n -p", "timeout": "-k -s", "stdbuf": "-i -o -e", "setsid": "",
}

// commandWords cuts what stands before the real command: keywords, assignments (X=1 rm p) and wrappers.
func commandWords(fields []string) []string {
	for len(fields) > 0 {
		if shellKeywords[fields[0]] || reAssignment.MatchString(fields[0]) {
			fields = fields[1:]
			continue
		}
		valued, wrapper := commandWrappers[baseCmd(fields[0])]
		if !wrapper {
			break
		}
		fields = skipWrapperArgs(fields[1:], valued)
	}
	return fields
}

// skipWrapperArgs drops a wrapper's own options, assignments and durations (timeout 5 cmd).
func skipWrapperArgs(args []string, valued string) []string {
	for len(args) > 0 {
		a := args[0]
		switch {
		case strings.Contains(" "+valued+" ", " "+a+" "):
			args = args[1:]
			if len(args) > 0 {
				args = args[1:]
			}
		case strings.HasPrefix(a, "-") || strings.Contains(a, "=") || reDuration.MatchString(a):
			args = args[1:]
		default:
			return args
		}
	}
	return args
}

// checkSimple refuses a simple command that writes, moves or deletes a protected path, or reads a
// hidden record. fields come from commandWords.
func (g *Guard) checkSimple(fields []string, dir, text string) error {
	head, args := baseCmd(fields[0]), fields[1:]
	if payload, opaque := encodedPayload(head, args); opaque {
		if payload == "" { // a base64 payload we cannot read is not allowed near the project
			return g.block("", core.M("guard.command"))
		}
		_, _, err := g.guard(payload, dir, true)
		return err
	}
	if payload := nestedPayload(head, args); payload != "" {
		_, _, err := g.guard(payload, dir, true)
		return err
	}
	if err := g.checkHiddenRead(head, args, dir); err != nil {
		return err
	}
	return g.checkWriter(head, args, dir, text)
}

// checkWriter applies the policy of the commands that change files.
func (g *Guard) checkWriter(head string, args []string, dir, text string) error {
	switch {
	case scriptingCommands[head]:
		if rel, ok := g.scriptProtected(dir, args); ok {
			return g.block(rel, core.M("guard.command"))
		}
		return g.refuseArgs(args, dir, text, core.M("guard.command"))
	case writingCommands[head]:
		if err := g.refuseArgs(args, dir, text, core.M("guard.command")); err != nil {
			return err
		}
		return g.checkTree(head, args, dir, text)
	case head == "sed" && hasInPlace(args):
		return g.refuseArgs(args, dir, text, core.M("guard.sed"))
	case head == "sort" && (hasFlag(args, "-o") || hasLongFlag(args, "--output")):
		return g.refuseArgs(args, dir, text, core.M("guard.command"))
	case head == "curl" || head == "wget":
		return g.checkDownload(args, dir, text)
	case head == "find" && findWrites(args):
		return g.refuseArgs(args, dir, text, core.M("guard.find"))
	case head == "git":
		return g.checkGit(args, dir, text)
	}
	return nil
}

// refuseArgs refuses the command when an argument names a protected path: bare, after key= or attached
// to a short flag (-oP).
func (g *Guard) refuseArgs(args []string, dir, text string, why core.Msg) error {
	for _, a := range args {
		cands := strings.Split(strings.TrimPrefix(a, "of="), "=")
		if len(a) > 2 && a[0] == '-' && a[1] != '-' {
			cands = append(cands, a[2:])
		}
		for _, cand := range cands {
			if rel, ok := g.Protects(dir, cand); ok {
				return g.block(rel, core.M("guard.with_command", "why", why, "command", core.Truncate(strings.TrimSpace(text), 120)))
			}
		}
	}
	return nil
}

// checkDownload refuses a download whose output file is protected.
func (g *Guard) checkDownload(args []string, dir, text string) error {
	target := outputTarget(args)
	if target == "" {
		return nil
	}
	return g.refuseArgs(append([]string{target}, args...), dir, text, core.M("guard.command"))
}
