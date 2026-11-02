package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleNotebook = `{
 "cells": [
  {"cell_type": "markdown", "id": "m1", "metadata": {}, "source": ["# Title\n", "text <b>\n"]},
  {"cell_type": "code", "id": "c1", "metadata": {"keep": 1}, "execution_count": 3, "source": "print(1)",
   "outputs": [{"output_type": "stream", "name": "stdout", "text": ["1\n"]},
    {"output_type": "error", "ename": "ValueError", "evalue": "bad", "traceback": ["line 1"]},
    {"output_type": "display_data", "data": {"text/plain": "<Figure>", "image/png": "aGVsbG8="}}]}
 ],
 "metadata": {"kernelspec": {"name": "python3"}},
 "nbformat": 4,
 "nbformat_minor": 5
}
`

func notebookFixture(t *testing.T) (*Session, *ToolContext, string) {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	p := filepath.Join(root, "nb.ipynb")
	if err := os.WriteFile(p, []byte(sampleNotebook), 0o644); err != nil {
		t.Fatal(err)
	}
	return s, &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}, p
}

func TestReadNotebookByCellWithOutputs(t *testing.T) {
	s, tc, _ := notebookFixture(t)
	out, isErr := readTool{}.Run(context.Background(), tc, json.RawMessage(`{"path":"nb.ipynb"}`))
	if isErr {
		t.Fatal(out)
	}
	for _, want := range []string{"cell 1 of 2 [markdown] id=m1", "# Title", "cell 2 of 2 [code] id=c1 execution=3", "print(1)", "[stream]", "ValueError: bad", "<Figure>", "image/png"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
	if len(s.pendingImages) != 1 {
		t.Fatal("the output image did not join the reply:", len(s.pendingImages))
	}
	only, _ := readTool{}.Run(context.Background(), tc, json.RawMessage(`{"path":"nb.ipynb","cells":"2"}`))
	if strings.Contains(only, "# Title") || !strings.Contains(only, "print(1)") {
		t.Fatal(only)
	}
	if out, isErr := (readTool{}).Run(context.Background(), tc, json.RawMessage(`{"path":"nb.ipynb","cells":"9"}`)); !isErr {
		t.Fatal("a cell that does not exist:", out)
	}
}

func TestNotebookEditActions(t *testing.T) {
	s, tc, p := notebookFixture(t)
	tool := notebookEditTool{}
	run := func(in string) (string, bool) { return tool.Run(context.Background(), tc, json.RawMessage(in)) }
	if out, isErr := run(`{"path":"nb.ipynb","action":"delete","cell":1}`); !isErr || !strings.Contains(out, "Read") {
		t.Fatal("edited an unread notebook:", out)
	}
	s.markRead(p)
	if out, isErr := run(`{"path":"nb.ipynb","action":"replace","cell_id":"c1","source":"print(2)\nprint(3)"}`); isErr {
		t.Fatal(out)
	}
	d := loadNB(t, p)
	var cell map[string]json.RawMessage
	_ = json.Unmarshal(d.cells[1], &cell)
	if sourceText(cell["source"]) != "print(2)\nprint(3)" || len(strings.TrimSpace(string(cell["outputs"]))) != 2 || !strings.Contains(string(cell["metadata"]), `"keep": 1`) {
		t.Fatalf("%s", d.cells[1])
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "<b>") {
		t.Fatal("markup in an untouched cell was escaped:", string(b))
	}
	if out, isErr := run(`{"path":"nb.ipynb","action":"insert","cell":2,"cell_type":"markdown","source":"note"}`); isErr {
		t.Fatal(out)
	}
	d = loadNB(t, p)
	var c notebookCell
	_ = json.Unmarshal(d.cells[1], &c)
	if len(d.cells) != 3 || c.Type != "markdown" || c.ID == "" {
		t.Fatalf("%d %+v", len(d.cells), c)
	}
	if out, isErr := run(`{"path":"nb.ipynb","action":"insert","source":"last"}`); isErr {
		t.Fatal(out)
	}
	if out, isErr := run(`{"path":"nb.ipynb","action":"delete","cell":1}`); isErr {
		t.Fatal(out)
	}
	d = loadNB(t, p)
	_ = json.Unmarshal(d.cells[0], &c)
	if len(d.cells) != 3 || sourceText(c.Source) != "note" {
		t.Fatalf("%d %s", len(d.cells), d.cells[0])
	}
	if out, isErr := run(`{"path":"nb.ipynb","action":"delete","cell":40}`); !isErr {
		t.Fatal("a cell that does not exist:", out)
	}
	if out, isErr := run(`{"path":".infuse/oracle/x.ipynb","action":"delete","cell":1}`); !isErr {
		t.Fatal("NotebookEdit reached the oracle:", out)
	}
}

func loadNB(t *testing.T, p string) *notebookDoc {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	d, err := parseNotebook(b)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// NotebookEdit is a write: the write gates judge the notebook as it would be.
func TestNotebookEditPassesTheWriteGates(t *testing.T) {
	if (notebookEditTool{}).Category() != "write" {
		t.Fatal("NotebookEdit must be a write tool")
	}
	_, tc, p := notebookFixture(t)
	input := json.RawMessage(`{"path":"nb.ipynb","action":"delete","cell":1}`)
	path, before, after, ok := editedContent(tc.Root, tc.Cwd, "NotebookEdit", input)
	if !ok || path != "nb.ipynb" || !strings.Contains(before, "# Title") || strings.Contains(after, "# Title") {
		t.Fatalf("%v %s", ok, path)
	}
	s := tc.S
	s.Mode = "plan"
	if got := s.permissionFor(notebookEditTool{}, input); got != "deny" {
		t.Fatal("plan mode allowed a notebook edit:", got, p)
	}
}

func TestReadPDFSaysWhatItCannotRead(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	tc := &ToolContext{S: s, Root: root, Cwd: root, Rules: &RuleSet{}}
	_ = os.WriteFile(filepath.Join(root, "bad.pdf"), []byte("not a pdf"), 0o644)
	if out, isErr := (readTool{}).Run(context.Background(), tc, json.RawMessage(`{"path":"bad.pdf"}`)); !isErr || !strings.Contains(out, "bad.pdf") {
		t.Fatal(out)
	}
	blank := "%PDF-1.4\n1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type/Pages/Kids[3 0 R]/Count 1>>\nendobj\n3 0 obj\n<</Type/Page/Parent 2 0 R/Resources<</XObject<</I 4 0 R>>>>>>\nendobj\n4 0 obj\n<</Type/XObject/Subtype/Image/Length 0>>\nstream\n\nendstream\nendobj\n"
	_ = os.WriteFile(filepath.Join(root, "scan.pdf"), []byte(blank), 0o644)
	out, isErr := readTool{}.Run(context.Background(), tc, json.RawMessage(`{"path":"scan.pdf"}`))
	if !isErr || !strings.Contains(out, "no text layer") {
		t.Fatal("a scan must not read as an empty success:", out)
	}
	text := "%PDF-1.4\n1 0 obj\n<</Type/Catalog/Pages 2 0 R>>\nendobj\n2 0 obj\n<</Type/Pages/Kids[3 0 R 5 0 R]/Count 2>>\nendobj\n3 0 obj\n<</Type/Page/Parent 2 0 R/Contents 4 0 R>>\nendobj\n4 0 obj\n<</Length 19>>\nstream\nBT (hello pdf) Tj ET\nendstream\nendobj\n5 0 obj\n<</Type/Page/Parent 2 0 R>>\nendobj\n"
	_ = os.WriteFile(filepath.Join(root, "doc.pdf"), []byte(text), 0o644)
	out, isErr = readTool{}.Run(context.Background(), tc, json.RawMessage(`{"path":"doc.pdf","pages":"1-2"}`))
	if isErr || !strings.Contains(out, "page 1 of 2") || !strings.Contains(out, "hello pdf") || !strings.Contains(out, "[page 2 has no text]") {
		t.Fatal(out)
	}
	if out, isErr := (readTool{}).Run(context.Background(), tc, json.RawMessage(`{"path":"doc.pdf","pages":"7"}`)); !isErr {
		t.Fatal("a range past the end:", out)
	}
}
