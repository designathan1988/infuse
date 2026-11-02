package filter

import "strings"

// Shell segmentation into simple commands.

// segScanner walks a command once, cutting it into simple commands. A ( subshell ) and a { group } at
// the start of a command are cut too, so the commands inside are read like any other; parentheses
// elsewhere ($(…), arrays, ${…}) stay inside their word and substitutions opens them separately.
type segScanner struct {
	cmd      string
	segs     []shellSeg
	last     int
	depth    int
	inS, inD bool
}

// splitSegments splits a command at top-level ; && || | & and newlines, and at the subshells and groups
// that open a command, respecting quotes, escapes and comments, recording each segment's offsets and
// the operator after it.
func splitSegments(cmd string) []shellSeg {
	s := &segScanner{cmd: cmd}
	for i := 0; i < len(cmd); i++ {
		i = s.step(i)
	}
	return append(s.segs, shellSeg{text: cmd[s.last:], start: s.last, end: len(cmd)})
}

// step handles the byte at i and returns the last byte it consumed.
func (s *segScanner) step(i int) int {
	c := s.cmd[i]
	switch {
	case s.inS:
		s.inS = c != '\''
	case s.inD:
		if c == '\\' {
			return i + 1
		}
		s.inD = c != '"'
	case c == '\\':
		if strings.HasPrefix(s.cmd[i+1:], "\r\n") {
			return i + 2
		}
		return i + 1
	case c == '\'':
		s.inS = true
	case c == '"':
		s.inD = true
	case c == '#' && s.startsWord(i):
		return s.endOfLine(i)
	case c == '(' || c == '{':
		s.open(i)
	case c == ')' || c == '}':
		s.close(i)
	case s.depth <= 0 && (c == ';' || c == '\n' || c == '|' || c == '&'):
		return s.separator(i)
	}
	return i
}

func (s *segScanner) startsWord(i int) bool {
	return i == s.last || strings.IndexByte(" \t\r\n", s.cmd[i-1]) >= 0
}

// endOfLine returns the byte before the newline that ends a comment: the newline still separates.
func (s *segScanner) endOfLine(i int) int {
	if nl := strings.IndexByte(s.cmd[i:], '\n'); nl >= 0 {
		return i + nl - 1
	}
	return len(s.cmd) - 1
}

func (s *segScanner) cut(end, next int, sep string) {
	s.segs = append(s.segs, shellSeg{text: s.cmd[s.last:end], start: s.last, end: end, sep: sep})
	s.last = next
}

func (s *segScanner) open(i int) {
	c := s.cmd[i]
	blank := i+1 < len(s.cmd) && strings.IndexByte(" \t\r\n", s.cmd[i+1]) >= 0
	if s.depth == 0 && s.atCommandStart(i) && (c == '(' || blank) {
		s.cut(i, i+1, string(c))
		return
	}
	s.depth++
}

func (s *segScanner) close(i int) {
	switch {
	case s.depth > 0:
		s.depth--
	case s.cmd[i] == ')':
		s.cut(i, i+1, ")")
	}
}

// atCommandStart reports whether what precedes i in the segment leaves the shell expecting a command.
func (s *segScanner) atCommandStart(i int) bool {
	words := strings.Fields(s.cmd[s.last:i])
	if len(words) == 0 {
		return true
	}
	return len(words) == 1 && shellKeywords[words[0]] || words[0] == "function" || strings.HasSuffix(words[len(words)-1], "()")
}

// separator cuts at an operator and returns its last byte. 2>&1, &>file and >| are redirections.
func (s *segScanner) separator(i int) int {
	cmd, c := s.cmd, s.cmd[i]
	if c == '&' && (i > 0 && (cmd[i-1] == '>' || cmd[i-1] == '<') || i+1 < len(cmd) && cmd[i+1] == '>') {
		return i
	}
	if c == '|' && i > 0 && cmd[i-1] == '>' {
		return i
	}
	sep, last := string(c), i
	if i+1 < len(cmd) && cmd[i+1] == c && (c == '&' || c == '|') {
		sep, last = sep+string(c), i+1
	}
	s.cut(i, last+1, sep)
	return last
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
// and leaving redirections out (they are handled separately). A backslash-newline joins two lines.
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
		case c == '\\' && strings.HasPrefix(seg[i+1:], "\r\n"):
			i += 2
		case c == '\\' && strings.HasPrefix(seg[i+1:], "\n"):
			i++
		case c == '\'':
			inS, has = true, true
		case c == '"':
			inD, has = true, true
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			flush()
		case c == '>' || c == '<':
			flush()
			i = skipRedirection(seg, i)
		default:
			cur.WriteByte(c)
			has = true
		}
	}
	flush()
	return out
}

// skipRedirection returns the last byte of the redirection whose operator starts at i.
func skipRedirection(seg string, i int) int {
	for i+1 < len(seg) && strings.IndexByte("><|& \t", seg[i+1]) >= 0 {
		i++
	}
	for i+1 < len(seg) && seg[i+1] != ' ' && seg[i+1] != '\t' {
		i++
	}
	return i
}

// substitutions returns the scripts a segment runs inside $(…), <(…), >(…) and backticks, which stay
// active within double quotes.
func substitutions(seg string) []string {
	var out []string
	inS, inD := false, false
	for i := 0; i < len(seg); i++ {
		c := seg[i]
		switch {
		case inS:
			inS = c != '\''
		case c == '\'' && !inD:
			inS = true
		case c == '\\':
			i++
		case c == '"':
			inD = !inD
		case c == '`':
			end := strings.IndexByte(seg[i+1:], '`')
			if end < 0 {
				return out
			}
			out = append(out, seg[i+1:i+1+end])
			i += end + 1
		case c == '(' && i > 0 && strings.IndexByte("$<>", seg[i-1]) >= 0:
			end := closingParen(seg, i)
			out = append(out, seg[i+1:end])
			i = end
		}
	}
	return out
}

// closingParen returns the offset of the ) matching the ( at open, or the end of seg.
func closingParen(seg string, open int) int {
	depth := 0
	for i := open; i < len(seg); i++ {
		switch seg[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(seg)
}
