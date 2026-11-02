package pdftext

// The PDF object syntax: a small tokenizer that turns bytes into Go values. Dictionaries are
// map[string]any, arrays []any, names name, strings []byte, numbers float64, references ref.

import (
	"bytes"
	"strconv"
)

type (
	name string
	ref  struct{ num, gen int }
	// keyword is a bare word in a content stream (an operator) or in the file's structure.
	keyword string
)

type lexer struct {
	b   []byte
	pos int
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t' || c == '\f' || c == 0
}

func isDelim(c byte) bool {
	return bytes.IndexByte([]byte("()<>[]{}/%"), c) >= 0
}

func (l *lexer) skipSpace() {
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		if c == '%' {
			for l.pos < len(l.b) && l.b[l.pos] != '\n' && l.b[l.pos] != '\r' {
				l.pos++
			}
			continue
		}
		if !isSpace(c) {
			return
		}
		l.pos++
	}
}

// next reads one value; ok is false at the end of the input or on a stray closing delimiter.
func (l *lexer) next() (any, bool) {
	l.skipSpace()
	if l.pos >= len(l.b) {
		return nil, false
	}
	switch c := l.b[l.pos]; {
	case c == '/':
		return l.readName(), true
	case c == '(':
		return l.readLiteral(), true
	case c == '[':
		l.pos++
		return l.readArray(), true
	case c == '<':
		if l.pos+1 < len(l.b) && l.b[l.pos+1] == '<' {
			l.pos += 2
			return l.readDict(), true
		}
		return l.readHex(), true
	case c == ']' || c == '>' || c == ')' || c == '{' || c == '}':
		l.pos++
		return nil, false
	}
	return l.readWord(), true
}

func (l *lexer) readName() name {
	l.pos++
	start := l.pos
	for l.pos < len(l.b) && !isSpace(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
		l.pos++
	}
	raw := l.b[start:l.pos]
	if bytes.IndexByte(raw, '#') < 0 {
		return name(raw)
	}
	var out []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && i+2 < len(raw) {
			if v, err := strconv.ParseUint(string(raw[i+1:i+3]), 16, 8); err == nil {
				out = append(out, byte(v))
				i += 2
				continue
			}
		}
		out = append(out, raw[i])
	}
	return name(out)
}

// readWord is a number, a reference (two integers and R), true/false/null or a keyword.
func (l *lexer) readWord() any {
	start := l.pos
	for l.pos < len(l.b) && !isSpace(l.b[l.pos]) && !isDelim(l.b[l.pos]) {
		l.pos++
	}
	if l.pos == start { // a delimiter that starts nothing: step over it
		l.pos++
		return keyword("")
	}
	w := string(l.b[start:l.pos])
	switch w {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	}
	f, err := strconv.ParseFloat(w, 64)
	if err != nil {
		return keyword(w)
	}
	if r, ok := l.tryRef(w); ok {
		return r
	}
	return f
}

// tryRef looks past an integer for "<gen> R".
func (l *lexer) tryRef(first string) (ref, bool) {
	num, err := strconv.Atoi(first)
	if err != nil {
		return ref{}, false
	}
	save := l.pos
	l.skipSpace()
	start := l.pos
	for l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '9' {
		l.pos++
	}
	if l.pos > start {
		gen, _ := strconv.Atoi(string(l.b[start:l.pos]))
		l.skipSpace()
		if l.pos < len(l.b) && l.b[l.pos] == 'R' && (l.pos+1 == len(l.b) || isSpace(l.b[l.pos+1]) || isDelim(l.b[l.pos+1])) {
			l.pos++
			return ref{num, gen}, true
		}
	}
	l.pos = save
	return ref{}, false
}

func (l *lexer) readArray() []any {
	out := []any{}
	for {
		l.skipSpace()
		if l.pos >= len(l.b) {
			return out
		}
		if l.b[l.pos] == ']' {
			l.pos++
			return out
		}
		v, ok := l.next()
		if ok {
			out = append(out, v)
		}
	}
}

func (l *lexer) readDict() map[string]any {
	out := map[string]any{}
	for {
		l.skipSpace()
		if l.pos >= len(l.b) {
			return out
		}
		if bytes.HasPrefix(l.b[l.pos:], []byte(">>")) {
			l.pos += 2
			return out
		}
		k, ok := l.next()
		if !ok {
			continue
		}
		key, isName := k.(name)
		if !isName {
			continue
		}
		if v, ok := l.next(); ok {
			out[string(key)] = v
		}
	}
}

func (l *lexer) readHex() []byte {
	l.pos++
	var out []byte
	hi, have := byte(0), false
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		if c == '>' {
			break
		}
		v, ok := hexVal(c)
		if !ok {
			continue
		}
		if have {
			out = append(out, hi<<4|v)
		} else {
			hi = v
		}
		have = !have
	}
	if have {
		out = append(out, hi<<4)
	}
	return out
}

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

func (l *lexer) readLiteral() []byte {
	l.pos++
	var out []byte
	depth := 1
	for l.pos < len(l.b) {
		c := l.b[l.pos]
		l.pos++
		switch c {
		case '\\':
			out = l.readEscape(out)
			continue
		case '(':
			depth++
		case ')':
			if depth--; depth == 0 {
				return out
			}
		}
		out = append(out, c)
	}
	return out
}

var literalEscapes = map[byte]byte{'n': '\n', 'r': '\r', 't': '\t', 'b': 8, 'f': 12}

func (l *lexer) readEscape(out []byte) []byte {
	if l.pos >= len(l.b) {
		return out
	}
	c := l.b[l.pos]
	l.pos++
	if v, ok := literalEscapes[c]; ok {
		return append(out, v)
	}
	if c == '\r' || c == '\n' { // a line continuation
		if c == '\r' && l.pos < len(l.b) && l.b[l.pos] == '\n' {
			l.pos++
		}
		return out
	}
	if c < '0' || c > '7' {
		return append(out, c)
	}
	v := int(c - '0')
	for i := 0; i < 2 && l.pos < len(l.b) && l.b[l.pos] >= '0' && l.b[l.pos] <= '7'; i++ {
		v = v*8 + int(l.b[l.pos]-'0')
		l.pos++
	}
	return append(out, byte(v))
}
