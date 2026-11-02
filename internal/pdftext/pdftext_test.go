package pdftext

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// classicPDF builds a small PDF: one object per string, one page per content stream.
func classicPDF(t *testing.T, contents []string, compress bool) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	kids := ""
	for i := range contents {
		kids += fmt.Sprintf("%d 0 R ", 10+i*2)
	}
	fmt.Fprintf(&b, "1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n")
	fmt.Fprintf(&b, "2 0 obj\n<</Type/Pages/Kids[%s]/Count %d/Resources<</Font<</F1 3 0 R>>>>>>\nendobj\n", kids, len(contents))
	fmt.Fprintf(&b, "3 0 obj\n<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>\nendobj\n")
	for i, c := range contents {
		fmt.Fprintf(&b, "%d 0 obj\n<</Type/Page/Parent 2 0 R/Contents %d 0 R>>\nendobj\n", 10+i*2, 11+i*2)
		data, filter := []byte(c), ""
		if compress {
			var z bytes.Buffer
			w := zlib.NewWriter(&z)
			_, _ = w.Write(data)
			_ = w.Close()
			data, filter = z.Bytes(), "/Filter/FlateDecode"
		}
		fmt.Fprintf(&b, "%d 0 obj\n<</Length %d%s>>\nstream\n", 11+i*2, len(data), filter)
		b.Write(data)
		b.WriteString("\nendstream\nendobj\n")
	}
	b.WriteString("trailer\n<</Root 1 0 R>>\n%%EOF\n")
	return b.Bytes()
}

func TestFlatePagesInOrder(t *testing.T) {
	pdf := classicPDF(t, []string{
		"BT /F1 12 Tf 72 700 Td (First page) Tj 0 -14 Td (second line) Tj ET",
		"BT /F1 12 Tf [(Sec)-10(ond)-300(page)] TJ ET",
		"BT /F1 12 Tf (caf\\351 \\(ok\\)) Tj ET",
	}, true)
	d, err := Open(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if d.NumPages() != 3 {
		t.Fatal(d.NumPages())
	}
	first, err := d.PageText(1)
	if err != nil || first != "First page\nsecond line" {
		t.Fatalf("%q %v", first, err)
	}
	if got, _ := d.PageText(2); got != "Second page" {
		t.Fatalf("%q", got)
	}
	if got, _ := d.PageText(3); got != "caf\u00e9 (ok)" {
		t.Fatalf("%q", got)
	}
}

func TestObjectStreamAndToUnicode(t *testing.T) {
	var body bytes.Buffer
	// the page tree lives in an object stream, the page's text in a composite font with a ToUnicode map
	objs := "<</Type/Catalog/Pages 2 0 R>> <</Type/Pages/Kids[3 0 R]/Count 1>> <</Type/Page/Parent 2 0 R/Contents 6 0 R/Resources<</Font<</F1 5 0 R>>>>>>"
	head := fmt.Sprintf("1 0 2 %d 3 %d ", strings.Index(objs, "<</Type/Pages"), strings.Index(objs, "<</Type/Page/"))
	data := head + objs
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	_, _ = w.Write([]byte(data))
	_ = w.Close()
	body.WriteString("%PDF-1.7\n")
	fmt.Fprintf(&body, "4 0 obj\n<</Type/ObjStm/N 3/First %d/Filter/FlateDecode/Length %d>>\nstream\n", len(head), z.Len())
	body.Write(z.Bytes())
	body.WriteString("\nendstream\nendobj\n")
	body.WriteString("5 0 obj\n<</Type/Font/Subtype/Type0/ToUnicode 7 0 R>>\nendobj\n")
	content := "BT /F1 12 Tf <00010002> Tj ET"
	fmt.Fprintf(&body, "6 0 obj\n<</Length %d>>\nstream\n%s\nendstream\nendobj\n", len(content), content)
	cmap := "1 begincodespacerange <0000> <FFFF> endcodespacerange\n1 beginbfrange <0001> <0002> <0048> endbfrange"
	fmt.Fprintf(&body, "7 0 obj\n<</Length %d>>\nstream\n%s\nendstream\nendobj\n", len(cmap), cmap)
	d, err := Open(body.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := d.PageText(1); err != nil || got != "HI" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestUnsupportedIsSaidNeverSilent(t *testing.T) {
	if _, err := Open([]byte("hello")); !errors.Is(err, ErrNotPDF) {
		t.Fatal(err)
	}
	enc := append(classicPDF(t, []string{"BT (x) Tj ET"}, false), []byte("\ntrailer\n<</Root 1 0 R/Encrypt 99 0 R>>\n")...)
	if _, err := Open(enc); !errors.Is(err, ErrEncrypted) {
		t.Fatal("encrypted document opened:", err)
	}
	if _, err := Open([]byte("%PDF-1.4\n1 0 obj\n<</Type/Font>>\nendobj\n")); !errors.Is(err, ErrNoPages) {
		t.Fatal(err)
	}
	d, err := Open(classicPDF(t, []string{"q 1 0 0 1 0 0 cm Q"}, false))
	if err != nil {
		t.Fatal(err)
	}
	var pe *PageError
	if _, err := d.PageText(1); !errors.As(err, &pe) || pe.Kind != KindEmpty {
		t.Fatal("an empty page must say so:", err)
	}
	if _, err := d.PageText(9); !errors.As(err, &pe) || pe.Kind != KindRange {
		t.Fatal(err)
	}
}

func TestParseRange(t *testing.T) {
	if a, b, err := ParseRange(" 10-20 "); err != nil || a != 10 || b != 20 {
		t.Fatal(a, b, err)
	}
	if a, b, err := ParseRange("3"); err != nil || a != 3 || b != 3 {
		t.Fatal(a, b, err)
	}
	for _, bad := range []string{"", "0", "5-2", "x"} {
		if _, _, err := ParseRange(bad); err == nil {
			t.Fatal(bad)
		}
	}
}
