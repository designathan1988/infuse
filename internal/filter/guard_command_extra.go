package filter

// Guard helpers for indirect writers: encoded PowerShell, inline script text and output flags.

import (
	"encoding/base64"
	"strings"
)

// psFlag matches a PowerShell parameter the way powershell.exe does: - or /, any case, and any prefix
// of the name (-e, -enc, -EncodedCommand; -c, -com, -Command).
func psFlag(a, name string) bool {
	if len(a) < 2 || a[0] != '-' && a[0] != '/' {
		return false
	}
	return strings.HasPrefix(name, strings.ToLower(a[1:]))
}

func isEncodedFlag(a string) bool {
	return psFlag(a, "encodedcommand") || strings.EqualFold(a, "-ec") || strings.EqualFold(a, "/ec")
}

// encodedPayload extracts a PowerShell -EncodedCommand script. opaque is true when the flag is
// present; payload is empty when the base64 cannot be decoded, which must not run silently.
func encodedPayload(head string, args []string) (payload string, opaque bool) {
	if head != "powershell" && head != "pwsh" {
		return "", false
	}
	for i, a := range args {
		if isEncodedFlag(a) {
			if i+1 < len(args) {
				return decodePowerShell(args[i+1]), true
			}
			return "", true
		}
		if flag, value, ok := strings.Cut(a, ":"); ok && isEncodedFlag(flag) {
			return decodePowerShell(value), true
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

// outputTarget returns where curl -o / wget -O writes ("" when it writes to stdout). Short options
// cluster and may carry their value attached: -o f, -of, -sSo f, -sSof.
func outputTarget(args []string) string {
	for i, a := range args {
		switch {
		case a == "--remote-name" || a == "--output-document" || a == "--output":
			return outputValue(args, i)
		case strings.HasPrefix(a, "--output=") || strings.HasPrefix(a, "--output-document="):
			if v := a[strings.IndexByte(a, '=')+1:]; v != "-" {
				return v
			}
		case strings.HasPrefix(a, "--") || !strings.HasPrefix(a, "-"):
		default:
			if at := strings.IndexAny(a, "oO"); at > 0 {
				if attached := a[at+1:]; attached != "" {
					return attached
				}
				return outputValue(args, i)
			}
		}
	}
	return ""
}

// outputValue reads the value after the option at i. -O without one names the file after the URL.
func outputValue(args []string, i int) string {
	if i+1 >= len(args) {
		return "(implied)"
	}
	if args[i+1] == "-" {
		return ""
	}
	return args[i+1]
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
