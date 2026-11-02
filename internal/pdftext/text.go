package pdftext

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// font is what decoding a string needs: the ToUnicode map, its code width, and whether the font is
// composite without any map (its codes are glyph ids and cannot become text).
type font struct {
	cmap       map[uint32]string
	width      int
	unreadable bool
}

// PageText extracts the text of page n (1-based). A page with nothing to read returns a *PageError that
// says why: drawn as an image, or set in a font without a Unicode map.
func (d *Doc) PageText(n int) (string, error) {
	if n < 1 || n > len(d.pages) {
		return "", &PageError{Page: n, Kind: KindRange}
	}
	page := d.pages[n-1]
	content, err := d.pageContent(page)
	if err != nil {
		return "", &PageError{Page: n, Kind: KindStream, Detail: err.Error()}
	}
	fonts := d.pageFonts(page)
	r := &textRun{fonts: fonts}
	r.interpret(content)
	text := strings.TrimSpace(r.out.String())
	if text != "" {
		return text, nil
	}
	switch {
	case r.skipped > 0:
		return "", &PageError{Page: n, Kind: KindFont}
	case d.pageHasImage(page):
		return "", &PageError{Page: n, Kind: KindImage}
	}
	return "", &PageError{Page: n, Kind: KindEmpty}
}

func (d *Doc) pageContent(page map[string]any) ([]byte, error) {
	var parts [][]byte
	for _, c := range d.contentRefs(page["Contents"]) {
		r, ok := c.(ref)
		if !ok {
			continue
		}
		o := d.objects[r.num]
		if o == nil || o.stream == nil {
			continue
		}
		dict, _ := o.value.(map[string]any)
		data, err := d.decode(dict, d.streamOf(o, dict))
		if err != nil {
			return nil, err
		}
		parts = append(parts, data)
	}
	return bytes.Join(parts, []byte("\n")), nil
}

// streamOf re-reads a stream whose /Length is a reference now that references resolve.
func (d *Doc) streamOf(o *object, dict map[string]any) []byte {
	if n, isRef := dict["Length"].(ref); isRef {
		if l := int(d.number(n)); l > 0 && l <= len(o.stream) {
			return o.stream[:l]
		}
	}
	return o.stream
}

func (d *Doc) contentRefs(v any) []any {
	if a := d.array(v); a != nil {
		return a
	}
	if v == nil {
		return nil
	}
	return []any{v}
}

func (d *Doc) pageFonts(page map[string]any) map[string]*font {
	out := map[string]*font{}
	res := d.dict(page["Resources"])
	if res == nil {
		return out
	}
	for k, v := range d.dict(res["Font"]) {
		out[k] = d.loadFont(d.dict(v))
	}
	return out
}

func (d *Doc) loadFont(dict map[string]any) *font {
	f := &font{width: 1}
	if dict == nil {
		return f
	}
	composite := dict["Subtype"] == name("Type0")
	if composite {
		f.width = 2
	}
	if r, ok := dict["ToUnicode"].(ref); ok {
		if o := d.objects[r.num]; o != nil && o.stream != nil {
			sd, _ := o.value.(map[string]any)
			if data, err := d.decode(sd, d.streamOf(o, sd)); err == nil {
				f.cmap = parseCMap(data)
			}
		}
	}
	f.unreadable = composite && len(f.cmap) == 0
	return f
}

func (d *Doc) pageHasImage(page map[string]any) bool {
	res := d.dict(page["Resources"])
	if res == nil {
		return false
	}
	for _, v := range d.dict(res["XObject"]) {
		if dict := d.dict(v); dict != nil && dict["Subtype"] == name("Image") {
			return true
		}
	}
	return false
}

// parseCMap reads the bfchar and bfrange sections of a ToUnicode stream.
func parseCMap(b []byte) map[uint32]string {
	out := map[uint32]string{}
	l := lexer{b: b}
	var stack []any
	for {
		v, ok := l.next()
		if !ok {
			if l.pos >= len(l.b) {
				return out
			}
			continue
		}
		kw, isKw := v.(keyword)
		if !isKw {
			stack = append(stack, v)
			continue
		}
		switch kw {
		case "endbfchar":
			for i := 0; i+1 < len(stack); i += 2 {
				src, ok1 := stack[i].([]byte)
				dst, ok2 := stack[i+1].([]byte)
				if ok1 && ok2 {
					out[codeOf(src)] = utf16Text(dst)
				}
			}
		case "endbfrange":
			bfRange(stack, out)
		}
		stack = stack[:0]
	}
}

func bfRange(stack []any, out map[uint32]string) {
	for i := 0; i+2 < len(stack); i += 3 {
		lo, ok1 := stack[i].([]byte)
		hi, ok2 := stack[i+1].([]byte)
		if !ok1 || !ok2 {
			continue
		}
		from, to := codeOf(lo), codeOf(hi)
		if to < from || to-from > 0xFFFF {
			continue
		}
		switch dst := stack[i+2].(type) {
		case []byte:
			for c := from; c <= to; c++ {
				out[c] = utf16Text(addLast(dst, int(c-from)))
			}
		case []any:
			for j, e := range dst {
				if b, ok := e.([]byte); ok && from+uint32(j) <= to {
					out[from+uint32(j)] = utf16Text(b)
				}
			}
		}
	}
}

func addLast(b []byte, n int) []byte {
	out := append([]byte{}, b...)
	for i := len(out) - 1; i >= 0 && n > 0; i-- {
		v := int(out[i]) + n
		out[i] = byte(v)
		n = v >> 8
	}
	return out
}

func codeOf(b []byte) uint32 {
	var v uint32
	for _, c := range b {
		v = v<<8 | uint32(c)
	}
	return v
}

func utf16Text(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}

// textRun interprets a content stream's text operators.
type textRun struct {
	out     strings.Builder
	fonts   map[string]*font
	font    *font
	lastY   float64
	haveY   bool
	skipped int // strings set in an unreadable font
}

func (r *textRun) newline() {
	if s := r.out.String(); s != "" && !strings.HasSuffix(s, "\n") {
		r.out.WriteByte('\n')
	}
}

// moveTo starts a new line when the text position's y changed.
func (r *textRun) moveTo(y float64) {
	if r.haveY && y != r.lastY {
		r.newline()
	}
	r.lastY, r.haveY = y, true
}

func (r *textRun) interpret(content []byte) {
	l := lexer{b: content}
	var args []any
	for {
		v, ok := l.next()
		if !ok {
			if l.pos >= len(content) {
				return
			}
			continue
		}
		kw, isKw := v.(keyword)
		if !isKw {
			args = append(args, v)
			continue
		}
		if kw == "ID" { // inline image data is binary: skip to its end marker
			skipInlineImage(&l)
		} else {
			r.apply(string(kw), args)
		}
		args = args[:0]
	}
}

func skipInlineImage(l *lexer) {
	if i := bytes.Index(l.b[l.pos:], []byte("EI")); i >= 0 {
		l.pos += i + 2
		return
	}
	l.pos = len(l.b)
}

func numArg(args []any, i int) float64 {
	if i < 0 || i >= len(args) {
		return 0
	}
	n, _ := args[i].(float64)
	return n
}

func (r *textRun) apply(op string, args []any) {
	switch op {
	case "Tf":
		if len(args) >= 2 {
			if n, ok := args[0].(name); ok {
				r.font = r.fonts[string(n)]
			}
		}
	case "Td", "TD":
		if numArg(args, 1) != 0 {
			r.newline()
		}
	case "T*":
		r.newline()
	case "Tm":
		r.moveTo(numArg(args, 5))
	case "Tj":
		r.show(args)
	case "'", "\"":
		r.newline()
		r.show(args)
	case "TJ":
		if len(args) > 0 {
			r.showArray(args[0])
		}
	case "ET":
		r.spaceBetween()
	}
}

func (r *textRun) spaceBetween() {
	if s := r.out.String(); s != "" && !strings.HasSuffix(s, " ") && !strings.HasSuffix(s, "\n") {
		r.out.WriteByte(' ')
	}
}

func (r *textRun) show(args []any) {
	if len(args) == 0 {
		return
	}
	if b, ok := args[len(args)-1].([]byte); ok {
		r.out.WriteString(r.decodeText(b))
	}
}

// showArray writes a TJ array; a large negative adjustment is a word space set by position.
func (r *textRun) showArray(v any) {
	items, ok := v.([]any)
	if !ok {
		return
	}
	for _, it := range items {
		switch e := it.(type) {
		case []byte:
			r.out.WriteString(r.decodeText(e))
		case float64:
			if e < -180 {
				r.spaceBetween()
			}
		}
	}
}

func (r *textRun) decodeText(b []byte) string {
	f := r.font
	if f == nil {
		f = &font{width: 1}
	}
	if f.unreadable {
		r.skipped++
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(b); i += f.width {
		end := min(i+f.width, len(b))
		code := codeOf(b[i:end])
		if s, ok := f.cmap[code]; ok {
			sb.WriteString(s)
		} else if f.width == 1 {
			sb.WriteRune(winAnsi(b[i]))
		}
	}
	return sb.String()
}

// winAnsi maps a single-byte code: the printable range of WinAnsi matches Unicode except 0x80-0x9F.
func winAnsi(c byte) rune {
	if r, ok := winAnsiHigh[c]; ok {
		return r
	}
	return rune(c)
}

var winAnsiHigh = map[byte]rune{
	0x80: 0x20AC, 0x82: 0x201A, 0x83: 0x0192, 0x84: 0x201E, 0x85: 0x2026, 0x86: 0x2020, 0x87: 0x2021,
	0x88: 0x02C6, 0x89: 0x2030, 0x8A: 0x0160, 0x8B: 0x2039, 0x8C: 0x0152, 0x8E: 0x017D, 0x91: 0x2018,
	0x92: 0x2019, 0x93: 0x201C, 0x94: 0x201D, 0x95: 0x2022, 0x96: 0x2013, 0x97: 0x2014, 0x98: 0x02DC,
	0x99: 0x2122, 0x9A: 0x0161, 0x9B: 0x203A, 0x9C: 0x0153, 0x9E: 0x017E, 0x9F: 0x0178,
}

// ParseRange reads "3" or "10-20" into first and last, 1-based and inclusive.
func ParseRange(s string) (int, int, error) {
	s = strings.TrimSpace(s)
	lo, hi, ranged := strings.Cut(s, "-")
	first, err := strconv.Atoi(strings.TrimSpace(lo))
	if err != nil || first < 1 {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	if !ranged {
		return first, first, nil
	}
	last, err := strconv.Atoi(strings.TrimSpace(hi))
	if err != nil || last < first {
		return 0, 0, fmt.Errorf("invalid range %q", s)
	}
	return first, last, nil
}
