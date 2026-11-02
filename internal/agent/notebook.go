package agent

// The Jupyter notebook format, as far as Read and NotebookEdit need it. A notebook is kept as raw JSON
// values: a cell the agent did not touch goes back byte for byte in meaning, with its outputs.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/pdftext"
	"path/filepath"
	"strings"
)

func isNotebook(abs string) bool { return strings.EqualFold(filepath.Ext(abs), ".ipynb") }

// notebookDoc is a parsed notebook: the document's keys, and its cells.
type notebookDoc struct {
	keys  map[string]json.RawMessage
	cells []json.RawMessage
}

// notebookCell is what the tools read of a cell.
type notebookCell struct {
	ID        string           `json:"id"`
	Type      string           `json:"cell_type"`
	Source    json.RawMessage  `json:"source"`
	Execution *int             `json:"execution_count"`
	Outputs   []notebookOutput `json:"outputs"`
}

type notebookOutput struct {
	Type   string                     `json:"output_type"`
	Name   string                     `json:"name"`
	Text   json.RawMessage            `json:"text"`
	Data   map[string]json.RawMessage `json:"data"`
	EName  string                     `json:"ename"`
	EValue string                     `json:"evalue"`
	Trace  []string                   `json:"traceback"`
}

var errNotNotebook = errors.New("not a version 4 notebook")

func parseNotebook(b []byte) (*notebookDoc, error) {
	d := &notebookDoc{}
	if err := json.Unmarshal(b, &d.keys); err != nil {
		return nil, err
	}
	raw, ok := d.keys["cells"]
	if !ok {
		return nil, errNotNotebook // nbformat 3 keeps its cells under worksheets
	}
	if err := json.Unmarshal(raw, &d.cells); err != nil {
		return nil, err
	}
	return d, nil
}

// encode writes the notebook as Jupyter does: sorted keys, one-space indent, a final newline.
func (d *notebookDoc) encode() ([]byte, error) {
	cells, err := marshalPlain(d.cells)
	if err != nil {
		return nil, err
	}
	d.keys["cells"] = cells
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", " ")
	if err := enc.Encode(d.keys); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func marshalPlain(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buf.Bytes()), nil
}

// sourceText joins a multi-line string: the format allows one string or a list of lines.
func sourceText(raw json.RawMessage) string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return one
	}
	var lines []string
	_ = json.Unmarshal(raw, &lines)
	return strings.Join(lines, "")
}

// sourceLines is text in the list form Jupyter writes: each line keeps its line break.
func sourceLines(text string) []string {
	if text == "" {
		return []string{}
	}
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ---------------------------------------------------------------- Read

const maxNotebookImages = 4 // images of outputs attached to one read

func (tc *ToolContext) readNotebook(abs string, b []byte, cells string) (string, bool) {
	d, err := parseNotebook(b)
	if err != nil {
		return tc.Say(M("read.nb_invalid", "file", relTo(tc.Root, abs), "error", err.Error())), true
	}
	total := len(d.cells)
	if total == 0 {
		tc.S.markRead(abs)
		return tc.Say(M("read.nb_empty", "file", relTo(tc.Root, abs))), false
	}
	first, last := 1, total
	if strings.TrimSpace(cells) != "" {
		if first, last, err = pdftext.ParseRange(cells); err != nil || first > total {
			return tc.Say(M("read.nb_bad_range", "cells", cells, "total", total)), true
		}
		last = min(last, total)
	}
	var sb strings.Builder
	images := 0
	for n := first; n <= last; n++ {
		if sb.Len() > maxToolOutput-4000 { // a long notebook is read in parts, like a long file
			sb.WriteString("\n" + tc.Say(M("read.nb_more", "total", total, "next", n)) + "\n")
			break
		}
		tc.renderCell(&sb, n, total, d.cells[n-1], &images)
	}
	tc.S.markRead(abs)
	return tc.clip(sb.String() + tc.readContext(abs)), false
}

func (tc *ToolContext) renderCell(sb *strings.Builder, n, total int, raw json.RawMessage, images *int) {
	var c notebookCell
	_ = json.Unmarshal(raw, &c)
	fmt.Fprintf(sb, "--- cell %d of %d [%s]", n, total, firstNonEmpty(c.Type, "unknown"))
	if c.ID != "" {
		fmt.Fprintf(sb, " id=%s", c.ID)
	}
	if c.Execution != nil {
		fmt.Fprintf(sb, " execution=%d", *c.Execution)
	}
	sb.WriteString(" ---\n" + strings.TrimRight(sourceText(c.Source), "\n") + "\n")
	for _, o := range c.Outputs {
		sb.WriteString("--- output [" + o.Type + "] ---\n")
		sb.WriteString(strings.TrimRight(tc.outputText(o, images), "\n") + "\n")
	}
}

// outputText is what an output says; images join the reply as images, up to a limit.
func (tc *ToolContext) outputText(o notebookOutput, images *int) string {
	switch {
	case o.Type == "error":
		return o.EName + ": " + o.EValue + "\n" + strings.Join(o.Trace, "\n")
	case len(o.Text) > 0:
		return sourceText(o.Text)
	}
	var parts []string
	if t, ok := o.Data["text/plain"]; ok {
		parts = append(parts, sourceText(t))
	}
	for _, mt := range []string{"image/png", "image/jpeg"} {
		if raw, ok := o.Data[mt]; ok {
			parts = append(parts, tc.attachOutputImage(mt, raw, images))
		}
	}
	return strings.Join(parts, "\n")
}

func (tc *ToolContext) attachOutputImage(mt string, raw json.RawMessage, images *int) string {
	data := strings.ReplaceAll(strings.TrimSpace(sourceText(raw)), "\n", "")
	size := base64.StdEncoding.DecodedLen(len(data))
	if *images >= maxNotebookImages || size > 5<<20 {
		return tc.Say(M("read.nb_image_skipped", "type", mt, "bytes", size))
	}
	*images++
	tc.S.Mu.Lock()
	tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: mt, Data: data})
	tc.S.Mu.Unlock()
	return tc.Say(M("read.nb_image_attached", "type", mt, "bytes", size))
}
