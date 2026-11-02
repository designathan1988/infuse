package agent

// Read of a PDF: the text of a page range, each page labelled. A document or a page that cannot be read
// says why; the call only succeeds when some text came out.

import (
	"errors"
	"fmt"
	"infuse/internal/pdftext"
	"path/filepath"
	"strings"
)

const (
	maxPDFPages = 20        // pages of one call
	maxPDFBytes = 200 << 20 // a larger file is refused before it is parsed
)

func isPDF(abs string) bool { return strings.EqualFold(filepath.Ext(abs), ".pdf") }

// pdfOpenMsgs says why a document cannot be opened.
var pdfOpenMsgs = []struct {
	err error
	id  string
}{
	{pdftext.ErrNotPDF, "read.pdf_invalid"},
	{pdftext.ErrEncrypted, "read.pdf_encrypted"},
	{pdftext.ErrNoPages, "read.pdf_no_pages"},
}

// pdfPageMsgs says why a page has no text.
var pdfPageMsgs = map[string]string{
	pdftext.KindRange:  "read.pdf_page_range",
	pdftext.KindImage:  "read.pdf_page_image",
	pdftext.KindFont:   "read.pdf_page_font",
	pdftext.KindEmpty:  "read.pdf_page_empty",
	pdftext.KindStream: "read.pdf_page_stream",
}

func (tc *ToolContext) readPDF(abs string, b []byte, pages string) (string, bool) {
	if len(b) > maxPDFBytes {
		return tc.Say(M("read.pdf_too_large", "bytes", len(b))), true
	}
	doc, err := pdftext.Open(b)
	if err != nil {
		for _, m := range pdfOpenMsgs {
			if errors.Is(err, m.err) {
				return tc.Say(M(m.id, "file", relTo(tc.Root, abs))), true
			}
		}
		return tc.Say(M("read.pdf_invalid", "file", relTo(tc.Root, abs))), true
	}
	first, last, more := pageWindow(doc.NumPages(), pages)
	if first == 0 {
		return tc.Say(M("read.pdf_bad_range", "pages", pages, "total", doc.NumPages(), "limit", maxPDFPages)), true
	}
	var sb strings.Builder
	readable := 0
	for n := first; n <= last; n++ {
		fmt.Fprintf(&sb, "--- page %d of %d ---\n", n, doc.NumPages())
		text, err := doc.PageText(n)
		if err != nil {
			sb.WriteString(tc.pdfPageNote(n, err) + "\n")
			continue
		}
		readable++
		sb.WriteString(text + "\n")
	}
	if more {
		sb.WriteString("\n" + tc.Say(M("read.pdf_more", "total", doc.NumPages(), "shown", last, "next", last+1)) + "\n")
	}
	if readable == 0 {
		return tc.Say(M("read.pdf_no_text", "first", first, "last", last)) + "\n" + sb.String(), true
	}
	tc.S.markRead(abs)
	return tc.clip(sb.String()), false
}

func (tc *ToolContext) pdfPageNote(n int, err error) string {
	var pe *pdftext.PageError
	if !errors.As(err, &pe) {
		return tc.Say(M("read.pdf_page_stream", "page", n, "detail", err.Error()))
	}
	return tc.Say(M(pdfPageMsgs[pe.Kind], "page", n, "detail", pe.Detail))
}

// pageWindow resolves the requested pages against the document: first is 0 when the request is invalid;
// more reports pages left after an unranged read.
func pageWindow(total int, pages string) (first, last int, more bool) {
	if strings.TrimSpace(pages) == "" {
		last = min(total, maxPDFPages)
		return 1, last, last < total
	}
	first, last, err := pdftext.ParseRange(pages)
	if err != nil || first > total || last-first+1 > maxPDFPages {
		return 0, 0, false
	}
	return first, min(last, total), false
}
