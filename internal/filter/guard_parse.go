package filter

import "strings"

// Shell segmentation into simple commands.

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
