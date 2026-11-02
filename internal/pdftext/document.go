// Package pdftext extracts the text of a PDF with the standard library alone, page by page. It reads
// classic and cross-reference-stream files, object streams and flate streams. Anything it cannot read
// (encryption, an unknown filter, a page drawn only as an image) comes back as an explicit error or
// note: an empty result is never silent.
package pdftext

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Unsupported documents, told apart so the caller can explain them.
var (
	ErrNotPDF    = errors.New("not a PDF file")
	ErrEncrypted = errors.New("the PDF is encrypted")
	ErrNoPages   = errors.New("no page tree was found")
)

type object struct {
	value  any
	stream []byte // the raw stream bytes, when the object has one
}

// Doc is a parsed PDF: every object of the file, and its pages in reading order.
type Doc struct {
	objects map[int]*object
	pages   []map[string]any
	seen    map[int]bool
}

var reObj = regexp.MustCompile(`(?:^|[\r\n\s])(\d+)[ \t\r\n]+(\d+)[ \t\r\n]+obj\b`)

// Open parses a PDF. The objects are found by scanning the file rather than through the
// cross-reference table, so damaged tables and both table formats read the same way; the last
// definition of an object wins, as an incremental update intends.
func Open(b []byte) (*Doc, error) {
	if i := bytes.Index(b, []byte("%PDF-")); i < 0 || i > 1024 {
		return nil, ErrNotPDF
	}
	d := &Doc{objects: map[int]*object{}, seen: map[int]bool{}}
	for _, m := range reObj.FindAllSubmatchIndex(b, -1) {
		num, _ := strconv.Atoi(string(b[m[2]:m[3]]))
		d.objects[num] = readObject(b, m[1])
	}
	if len(d.objects) == 0 {
		return nil, ErrNotPDF
	}
	if bytes.Contains(b, []byte("/Encrypt")) && d.hasEncryption(b) {
		return nil, ErrEncrypted
	}
	d.expandObjectStreams()
	d.collectPages()
	if len(d.pages) == 0 {
		return nil, ErrNoPages
	}
	return d, nil
}

// NumPages is the number of pages of the document.
func (d *Doc) NumPages() int { return len(d.pages) }

// hasEncryption looks at the trailers (classic or stream) rather than at the bytes alone: a document
// about PDF encryption may say /Encrypt in its text.
func (d *Doc) hasEncryption(b []byte) bool {
	for _, o := range d.objects {
		if dict, ok := o.value.(map[string]any); ok && dict["Encrypt"] != nil {
			return true
		}
	}
	idx := 0
	for {
		i := bytes.Index(b[idx:], []byte("trailer"))
		if i < 0 {
			return false
		}
		idx += i + len("trailer")
		l := lexer{b: b, pos: idx}
		if v, ok := l.next(); ok {
			if dict, ok := v.(map[string]any); ok && dict["Encrypt"] != nil {
				return true
			}
		}
	}
}

func readObject(b []byte, pos int) *object {
	l := lexer{b: b, pos: pos}
	v, _ := l.next()
	o := &object{value: v}
	dict, ok := v.(map[string]any)
	if !ok {
		return o
	}
	l.skipSpace()
	if !bytes.HasPrefix(b[l.pos:], []byte("stream")) {
		return o
	}
	start := l.pos + len("stream")
	if start < len(b) && b[start] == '\r' {
		start++
	}
	if start < len(b) && b[start] == '\n' {
		start++
	}
	o.stream = rawStream(b, start, dict)
	return o
}

// rawStream takes /Length when it lands on endstream, else the next endstream: /Length is often a
// reference this early scan cannot follow yet.
func rawStream(b []byte, start int, dict map[string]any) []byte {
	if n, ok := dict["Length"].(float64); ok {
		end := start + int(n)
		if end <= len(b) && end >= start {
			rest := b[end:min(end+16, len(b))]
			if bytes.HasPrefix(bytes.TrimLeft(rest, "\r\n "), []byte("endstream")) {
				return b[start:end]
			}
		}
	}
	if i := bytes.Index(b[start:], []byte("endstream")); i >= 0 {
		return bytes.TrimRight(b[start:start+i], "\r\n")
	}
	return b[start:]
}

// expandObjectStreams reads the objects packed inside /ObjStm streams (PDF 1.5 and later). An object
// outside a stream is the file's own definition and stays; a packed one fills the gaps.
func (d *Doc) expandObjectStreams() {
	nums := make([]int, 0, len(d.objects))
	for n := range d.objects {
		nums = append(nums, n)
	}
	for _, n := range nums {
		o := d.objects[n]
		dict, ok := o.value.(map[string]any)
		if !ok || dict["Type"] != name("ObjStm") {
			continue
		}
		data, err := d.decode(dict, o.stream)
		if err != nil {
			continue
		}
		first := int(d.number(dict["First"]))
		count := int(d.number(dict["N"]))
		if first <= 0 || first > len(data) {
			continue
		}
		head := lexer{b: data[:first]}
		for i := 0; i < count; i++ {
			numV, ok1 := head.next()
			offV, ok2 := head.next()
			num, isNum := numV.(float64)
			off, isOff := offV.(float64)
			if !ok1 || !ok2 || !isNum || !isOff {
				break
			}
			if _, have := d.objects[int(num)]; have || first+int(off) > len(data) {
				continue
			}
			l := lexer{b: data, pos: first + int(off)}
			if v, ok := l.next(); ok {
				d.objects[int(num)] = &object{value: v}
			}
		}
	}
}

// resolve follows references to the value they name.
func (d *Doc) resolve(v any) any {
	for i := 0; i < 32; i++ {
		r, ok := v.(ref)
		if !ok {
			return v
		}
		o := d.objects[r.num]
		if o == nil {
			return nil
		}
		v = o.value
	}
	return nil
}

func (d *Doc) dict(v any) map[string]any {
	m, _ := d.resolve(v).(map[string]any)
	return m
}

func (d *Doc) array(v any) []any {
	a, _ := d.resolve(v).([]any)
	return a
}

func (d *Doc) number(v any) float64 {
	n, _ := d.resolve(v).(float64)
	return n
}

// collectPages walks the page tree from the catalog; when the catalog cannot be found, /Pages nodes
// without a parent are the roots.
func (d *Doc) collectPages() {
	for _, o := range d.objects {
		if dict, ok := o.value.(map[string]any); ok && dict["Type"] == name("Catalog") {
			d.walkPages(dict["Pages"], nil)
		}
	}
	if len(d.pages) > 0 {
		return
	}
	for _, o := range d.objects {
		if dict, ok := o.value.(map[string]any); ok && dict["Type"] == name("Pages") && dict["Parent"] == nil {
			d.walkPages(dict, nil)
		}
	}
}

// inherited are the attributes a page takes from its ancestors.
var inherited = []string{"Resources", "MediaBox", "CropBox", "Rotate"}

func (d *Doc) walkPages(v any, parent map[string]any) {
	if r, ok := v.(ref); ok {
		if d.seen[r.num] { // a cycle in a damaged tree
			return
		}
		d.seen[r.num] = true
	}
	node := d.dict(v)
	if node == nil {
		return
	}
	for _, k := range inherited {
		if node[k] == nil && parent != nil {
			node[k] = parent[k]
		}
	}
	if node["Kids"] == nil {
		if node["Type"] == name("Page") || node["Contents"] != nil {
			d.pages = append(d.pages, node)
		}
		return
	}
	for _, kid := range d.array(node["Kids"]) {
		d.walkPages(kid, node)
	}
}

// Why a page has no text.
const (
	KindRange  = "range"  // no such page
	KindImage  = "image"  // no text layer: the page is drawn as an image
	KindFont   = "font"   // set in a font with no Unicode map
	KindEmpty  = "empty"  // nothing on the page to read
	KindStream = "stream" // its content stream could not be decoded
)

// PageError says why a page has no text; Detail carries a stream's own error.
type PageError struct {
	Page   int
	Kind   string
	Detail string
}

func (e *PageError) Error() string { return fmt.Sprintf("page %d: %s %s", e.Page, e.Kind, e.Detail) }
