package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Read: text, images and PDFs with line ranges.

// ---------------------------------------------------------------- Read

type readTool struct{}

func (readTool) Category() string { return "read" }
func (readTool) Spec() ToolSpec {
	return ToolSpec{Name: "Read", Description: "Reads a project file with line numbers. Use offset/limit for large files. Images are shown to you. PDFs come back as the text of each page: pass pages (e.g. \"3\" or \"10-20\", at most 20 at a time). Jupyter notebooks come back cell by cell with their outputs: pass cells (e.g. \"1-5\") for part of one.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","description":"First line (1-based)"},"limit":{"type":"integer","description":"Number of lines"},"pages":{"type":"string","description":"PDF only: page range, e.g. 3 or 10-20"},"cells":{"type":"string","description":"Notebook only: cell range, e.g. 2 or 1-5"}},"required":["path"]}`)}
}

func (readTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path          string `json:"path"`
		Offset, Limit int
		Pages, Cells  string
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return tc.SayErr(err), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if mt := imageType(abs); mt != "" {
		return tc.readImage(abs, mt, b)
	}
	if isPDF(abs) {
		return tc.readPDF(abs, b, in.Pages)
	}
	if isNotebook(abs) {
		return tc.readNotebook(abs, b, in.Cells)
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return tc.Say(M("read.binary", "bytes", len(b))), true
	}
	lines := strings.Split(string(b), "\n")
	start := in.Offset
	if start < 1 {
		start = 1
	}
	end := len(lines)
	if in.Limit > 0 && start-1+in.Limit < end {
		end = start - 1 + in.Limit
	}
	if start > len(lines) {
		return "", false
	}
	var sb strings.Builder
	shown := start - 1
	for i := start - 1; i < end; i++ {
		if sb.Len() > maxToolOutput-2000 { // a long file is read in parts, never with its middle cut out
			break
		}
		fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i])
		shown = i + 1
	}
	if shown < end {
		sb.WriteString("\n" + tc.Say(M("read.continues", "lines", len(lines), "offset", shown+1)) + "\n")
	}
	tc.S.noteCoverage(abs, len(lines), start, shown)
	tc.S.markRead(abs)
	out := sb.String() + tc.readContext(abs)
	return tc.clip(out), false
}

// readImage attaches an image: the model sees it itself.
func (tc *ToolContext) readImage(abs, mt string, b []byte) (string, bool) {
	if len(b) > 5<<20 {
		return tc.Say(M("read.image_too_large", "bytes", len(b))), true
	}
	tc.S.Mu.Lock()
	tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: mt, Data: base64.StdEncoding.EncodeToString(b)})
	tc.S.Mu.Unlock()
	tc.S.markRead(abs)
	return tc.Say(M("read.image_attached", "file", relTo(tc.Root, abs), "bytes", len(b))), false
}

// readContext is what infuse adds to a file's content: its dependents and the rules of its folder.
func (tc *ToolContext) readContext(abs string) string {
	var out string
	if m := LoadMap(tc.Root); m != nil {
		if d := DependentsMsg(m, relTo(tc.Root, abs)); !d.Empty() {
			out += "\n" + tc.Say(d)
		}
	}
	if inst := tc.S.localInstructions(filepath.Dir(abs)); inst != "" {
		out += "\n\n" + tc.Say(M("read.localInstructions")) + "\n" + inst
	}
	return out
}
