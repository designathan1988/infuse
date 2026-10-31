package main

// Tolerant heredoc parser for shell commands emitted by coding agents.
//
// Deliberate tolerances (these are exactly where agents break):
//   - indented terminator, or terminator with trailing whitespace
//   - CRLF line endings
//   - missing terminator at the end of the command (end of text closes the heredoc)
//   - quoted or unquoted delimiter

import (
	"regexp"
	"strings"
)

type Heredoc struct {
	OpStart, OpEnd     int // position of `<<` and position right after the delimiter token
	BodyStart, BodyEnd int
	BlockEnd           int // end of the whole block, including the terminator line
	Delimiter          string
	Quoted, StripTabs  bool
	Body               string
	TerminatorFound    bool
}

func isDelimEnd(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == ';' || c == '|' || c == '&' || c == '<' || c == '>' || c == ')'
}

// FindHeredocs scans the command outside quotes and finds `<<` / `<<-` operators (not `<<<`).
func FindHeredocs(cmd string) []Heredoc {
	type op struct {
		start, end int
		delim      string
		quoted     bool
		strip      bool
	}
	var ops []op
	n := len(cmd)
	inSingle, inDouble := false, false
	for i := 0; i < n; {
		c := cmd[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
			i++
		case inDouble:
			if c == '\\' {
				i += 2
				continue
			}
			if c == '"' {
				inDouble = false
			}
			i++
		case c == '\\':
			i += 2
		case c == '\'':
			inSingle = true
			i++
		case c == '"':
			inDouble = true
			i++
		case c == '#':
			for i < n && cmd[i] != '\n' {
				i++
			}
		case c == '<' && i+1 < n && cmd[i+1] == '<' && (i+2 >= n || cmd[i+2] != '<') && (i == 0 || cmd[i-1] != '<'):
			start := i
			i += 2
			strip := false
			if i < n && cmd[i] == '-' {
				strip = true
				i++
			}
			for i < n && (cmd[i] == ' ' || cmd[i] == '\t') {
				i++
			}
			quoted := false
			var delim strings.Builder
			if i < n && (cmd[i] == '\'' || cmd[i] == '"') {
				q := cmd[i]
				quoted = true
				i++
				for i < n && cmd[i] != q {
					delim.WriteByte(cmd[i])
					i++
				}
				if i < n { // the closing quote (an unterminated one ends at the end of the command)
					i++
				}
			} else {
				for i < n && !isDelimEnd(cmd[i]) {
					if cmd[i] == '\\' {
						quoted = true
						i++
						if i >= n {
							break
						}
					}
					delim.WriteByte(cmd[i])
					i++
				}
			}
			if delim.Len() == 0 {
				continue
			}
			ops = append(ops, op{start, i, delim.String(), quoted, strip})
		default:
			i++
		}
	}
	if len(ops) == 0 {
		return nil
	}

	var result []Heredoc
	cursor := 0      // end of the previous heredoc block
	opsLineEnd := -2 // newline ending the command line of the previous operator
	for _, o := range ops {
		h := Heredoc{OpStart: o.start, OpEnd: o.end, Delimiter: o.delim, Quoted: o.quoted, StripTabs: o.strip}
		var bodyStart int
		if o.start < opsLineEnd {
			bodyStart = cursor // same command line as the previous operator
		} else {
			nl := strings.IndexByte(cmd[o.end:], '\n')
			if nl == -1 {
				h.BodyStart, h.BodyEnd, h.BlockEnd = n, n, n
				result = append(result, h)
				cursor, opsLineEnd = n, n
				continue
			}
			opsLineEnd = o.end + nl
			bodyStart = opsLineEnd + 1
		}
		var lines []string
		pos := bodyStart
		h.BodyStart = bodyStart
		h.BodyEnd, h.BlockEnd = n, n
		for pos <= n {
			nl := -1
			if pos < n {
				nl = strings.IndexByte(cmd[pos:], '\n')
			}
			var raw string
			if nl == -1 {
				raw = cmd[pos:]
			} else {
				raw = cmd[pos : pos+nl]
			}
			line := strings.TrimSuffix(raw, "\r")
			probe := line
			if o.strip {
				probe = strings.TrimLeft(line, "\t")
			}
			if strings.TrimSpace(probe) == o.delim {
				h.TerminatorFound = true
				h.BodyEnd = pos
				if nl == -1 {
					h.BlockEnd = n
				} else {
					h.BlockEnd = pos + nl + 1
				}
				break
			}
			if o.strip {
				line = strings.TrimLeft(line, "\t")
			}
			lines = append(lines, line)
			if nl == -1 {
				break
			}
			pos += nl + 1
		}
		if len(lines) > 0 {
			h.Body = strings.Join(lines, "\n") + "\n"
		}
		result = append(result, h)
		cursor = h.BlockEnd
	}
	return result
}

type FileWriteIntent struct {
	Path   string
	Append bool
}

var (
	pathTok       = `('[^']*'|"[^"]*"|\S+)`
	reCatThenHd   = regexp.MustCompile(`^cat(?:\s+-[A-Za-z]+)*\s*(>>?)\s*` + pathTok + `\s*<<.*$`)
	reCatHdThen   = regexp.MustCompile(`^cat(?:\s+-[A-Za-z]+)*\s*<<-?\s*['"\\]?\w+['"]?\s*(>>?)\s*` + pathTok + `\s*$`)
	reTee         = regexp.MustCompile(`^(?:sudo\s+)?tee\s+(-a\s+)?` + pathTok + `\s*(?:>\s*/dev/null\s*)?<<.*$`)
	reLiteralHint = regexp.MustCompile("`|\\$\\(|\\$\\{")
)

// DetectFileWrite recognizes, on the command line holding the heredoc, the intent to write a file.
func DetectFileWrite(commandLine string) *FileWriteIntent {
	line := strings.TrimSpace(commandLine)
	if m := reCatThenHd.FindStringSubmatch(line); m != nil {
		return &FileWriteIntent{unquote(m[2]), m[1] == ">>"}
	}
	if m := reCatHdThen.FindStringSubmatch(line); m != nil {
		return &FileWriteIntent{unquote(m[2]), m[1] == ">>"}
	}
	if m := reTee.FindStringSubmatch(line); m != nil {
		return &FileWriteIntent{unquote(m[2]), m[1] != ""}
	}
	return nil
}

func unquote(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

// CommandLineOf returns the command line (line start to newline) on which the operator appears.
func CommandLineOf(cmd string, h Heredoc) (start, end int, text string) {
	start = strings.LastIndexByte(cmd[:h.OpStart], '\n') + 1
	end = len(cmd)
	if nl := strings.IndexByte(cmd[h.OpEnd:], '\n'); nl != -1 {
		end = h.OpEnd + nl
	}
	return start, end, cmd[start:end]
}

// LooksLikeLiteralIntent: signs that the agent wanted literal content but left the delimiter unquoted.
func LooksLikeLiteralIntent(body string) bool { return reLiteralHint.MatchString(body) }
