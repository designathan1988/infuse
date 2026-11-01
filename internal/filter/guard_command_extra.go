package filter

// Guard helpers for indirect writers: encoded PowerShell, inline script text and output flags.

import (
	"encoding/base64"
	"strings"
)

// CommandMayWrite reports whether a shell command can change the filesystem: heredocs into files,
// redirections, writing commands, inline scripts and output flags. It is a predicate for gates that
// must know whether a command is a write without running it.
func CommandMayWrite(cmd string) bool {
	hs := FindHeredocs(cmd)
	for _, h := range hs {
		_, _, line := CommandLineOf(cmd, h)
		if DetectFileWrite(line) != nil {
			return true
		}
	}
	for _, m := range ReRedir.FindAllStringSubmatch(cmd, -1) {
		target := strings.Trim(m[3], `"'`)
		if target != "" && target != "/dev/null" && !strings.EqualFold(target, "NUL") {
			return true
		}
	}
	for _, sg := range splitSegments(maskHeredocBodies(cmd, hs)) {
		fields := strings.Fields(sg.text)
		if len(fields) == 0 {
			continue
		}
		head, args := baseCmd(fields[0]), fields[1:]
		switch {
		case writingCommands[head], scriptingCommands[head]:
			return true
		case head == "sed" && hasInPlace(args):
			return true
		case head == "sort" && (hasFlag(args, "-o") || hasLongFlag(args, "--output")):
			return true
		case (head == "curl" || head == "wget") && outputTarget(args) != "":
			return true
		case head == "git" && len(args) > 0 && gitWriting[args[0]]:
			return true
		}
	}
	return false
}

// encodedPayload extracts a PowerShell -EncodedCommand script. opaque is true when the flag is
// present; payload is empty when the base64 cannot be decoded, which must not run silently.
func encodedPayload(head string, args []string) (payload string, opaque bool) {
	if head != "powershell" && head != "pwsh" {
		return "", false
	}
	for i, a := range args {
		if strings.EqualFold(a, "-encodedcommand") || strings.EqualFold(a, "-enc") {
			if i+1 < len(args) {
				return decodePowerShell(args[i+1]), true
			}
			return "", true
		}
		for _, flag := range []string{"-encodedcommand:", "-enc:"} {
			if len(a) > len(flag) && strings.EqualFold(a[:len(flag)], flag) {
				return decodePowerShell(a[len(flag):]), true
			}
		}
	}
	return "", false
}

// decodePowerShell decodes the UTF-16LE base64 payload PowerShell accepts.
func decodePowerShell(s string) string {
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return ""
	}
	out := make([]rune, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		out = append(out, rune(b[i])|rune(b[i+1])<<8)
	}
	return string(out)
}

// scriptProtected finds a protected path named inside an inline script (node -e, python -c).
func (g *Guard) scriptProtected(dir string, args []string) (string, bool) {
	for _, a := range args {
		for _, tok := range strings.FieldsFunc(a, func(r rune) bool {
			return strings.ContainsRune(" \t\r\n'\"`()[]{},;=+<>|&", r)
		}) {
			if rel, ok := g.Protects(dir, tok); ok {
				return rel, true
			}
		}
	}
	return "", false
}

// outputTarget returns where curl -o / wget -O writes ("" when it writes to stdout).
func outputTarget(args []string) string {
	for i, a := range args {
		switch {
		case a == "-O" || a == "--remote-name" || a == "--output-document" || a == "--output":
			if i+1 < len(args) {
				if args[i+1] == "-" {
					return ""
				}
				return args[i+1]
			}
			return "(implied)"
		case strings.HasPrefix(a, "--output="):
			if v := strings.TrimPrefix(a, "--output="); v != "-" {
				return v
			}
		case a == "-o" && i+1 < len(args):
			if args[i+1] != "-" {
				return args[i+1]
			}
		}
	}
	return ""
}

// hasInPlace recognizes both -i and --in-place, with or without an attached value.
func hasInPlace(args []string) bool {
	for _, f := range args {
		if f == "-i" || strings.HasPrefix(f, "-i") && !strings.HasPrefix(f, "--") {
			return true
		}
		if f == "--in-place" || strings.HasPrefix(f, "--in-place=") {
			return true
		}
	}
	return false
}

func hasLongFlag(fields []string, flag string) bool {
	for _, f := range fields {
		if f == flag || strings.HasPrefix(f, flag+"=") {
			return true
		}
	}
	return false
}
