package agent

// NotebookEdit: replaces, inserts or deletes one cell of a Jupyter notebook. It is a write tool: it
// passes every write gate (editedContent gives them the notebook as it would be), needs the file read
// first, checkpoints it and replaces it in one rename.

import (
	"context"
	"encoding/json"
	"infuse/internal/core"
	"os"
	"strings"
)

type notebookEditTool struct{}

func (notebookEditTool) Category() string { return "write" }
func (notebookEditTool) Spec() ToolSpec {
	return ToolSpec{Name: "NotebookEdit", Description: "Edits one cell of a Jupyter notebook (.ipynb): action=replace changes its source, action=insert adds a new cell before the given one (or at the end), action=delete removes it. Name the cell by cell (1-based, as Read numbers them) or cell_id. Read the notebook first. Replacing a code cell clears its outputs.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"action":{"type":"string","enum":["replace","insert","delete"]},"cell":{"type":"integer","description":"Cell number (1-based)"},"cell_id":{"type":"string"},"source":{"type":"string","description":"The cell's new source"},"cell_type":{"type":"string","enum":["code","markdown","raw"]}},"required":["path","action"]}`)}
}

type notebookEditInput struct {
	Path     string `json:"path"`
	Action   string `json:"action"`
	Cell     int    `json:"cell"`
	CellID   string `json:"cell_id"`
	Source   string `json:"source"`
	CellType string `json:"cell_type"`
}

func (notebookEditTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in notebookEditInput
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return tc.SayErr(err), true
	}
	if !isNotebook(abs) {
		return tc.Say(M("nb.not_notebook", "file", in.Path)), true
	}
	if !tc.S.wasRead(abs) {
		return tc.Say(M("edit.unread")), true
	}
	before, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	after, why := applyNotebookEdit(before, in)
	if !why.Empty() {
		return tc.Say(why), true
	}
	tc.S.checkpointBeforeWrite(abs)
	if err := core.WriteFileAtomic(abs, after); err != nil {
		return err.Error(), true
	}
	lines := append([]string{tc.Say(M("nb.done", "action", in.Action, "file", relTo(tc.Root, abs)))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// applyNotebookEdit returns the notebook after the edit, or why the edit cannot be made.
func applyNotebookEdit(before []byte, in notebookEditInput) ([]byte, Msg) {
	d, err := parseNotebook(before)
	if err != nil {
		return nil, M("nb.invalid", "error", err.Error())
	}
	at, why := d.locate(in)
	if !why.Empty() {
		return nil, why
	}
	switch in.Action {
	case "replace":
		cell, why := replacedCell(d.cells[at], in)
		if !why.Empty() {
			return nil, why
		}
		d.cells[at] = cell
	case "insert":
		cell, err := newCell(in, d.needsCellID())
		if err != nil {
			return nil, M("nb.invalid", "error", err.Error())
		}
		d.cells = append(d.cells[:at:at], append([]json.RawMessage{cell}, d.cells[at:]...)...)
	case "delete":
		d.cells = append(d.cells[:at:at], d.cells[at+1:]...)
	default:
		return nil, M("nb.bad_action", "action", in.Action)
	}
	out, err := d.encode()
	if err != nil {
		return nil, M("nb.invalid", "error", err.Error())
	}
	return out, Msg{}
}

// locate turns cell or cell_id into an index. An insert may name the position after the last cell, and
// with no cell named it appends.
func (d *notebookDoc) locate(in notebookEditInput) (int, Msg) {
	if in.CellID != "" {
		for i, raw := range d.cells {
			var c notebookCell
			if json.Unmarshal(raw, &c) == nil && c.ID == in.CellID {
				return i, Msg{}
			}
		}
		return 0, M("nb.no_cell", "cell", in.CellID, "total", len(d.cells))
	}
	limit := len(d.cells)
	if in.Action == "insert" {
		if in.Cell == 0 {
			return limit, Msg{}
		}
		limit++
	}
	if in.Cell < 1 || in.Cell > limit {
		return 0, M("nb.no_cell", "cell", in.Cell, "total", len(d.cells))
	}
	return in.Cell - 1, Msg{}
}

// needsCellID: notebooks of format 4.5 and later give every cell an id.
func (d *notebookDoc) needsCellID() bool {
	var major, minor int
	_ = json.Unmarshal(d.keys["nbformat"], &major)
	_ = json.Unmarshal(d.keys["nbformat_minor"], &minor)
	return major > 4 || major == 4 && minor >= 5
}

// replacedCell changes the source (and type, when given) and leaves the rest of the cell as it was.
func replacedCell(raw json.RawMessage, in notebookEditInput) (json.RawMessage, Msg) {
	var cell map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cell); err != nil {
		return nil, M("nb.invalid", "error", err.Error())
	}
	source, _ := marshalPlain(sourceLines(in.Source))
	cell["source"] = source
	var kind string
	_ = json.Unmarshal(cell["cell_type"], &kind)
	if in.CellType != "" && in.CellType != kind {
		kind = in.CellType
		cell["cell_type"], _ = marshalPlain(kind)
	}
	if kind == "code" { // the old outputs belong to the old source
		cell["outputs"] = json.RawMessage("[]")
		cell["execution_count"] = json.RawMessage("null")
	} else {
		delete(cell, "outputs")
		delete(cell, "execution_count")
	}
	out, err := marshalPlain(cell)
	if err != nil {
		return nil, M("nb.invalid", "error", err.Error())
	}
	return out, Msg{}
}

func newCell(in notebookEditInput, withID bool) (json.RawMessage, error) {
	kind := firstNonEmpty(in.CellType, "code")
	cell := map[string]any{"cell_type": kind, "metadata": map[string]any{}, "source": sourceLines(in.Source)}
	if kind == "code" {
		cell["outputs"] = []any{}
		cell["execution_count"] = nil
	}
	if withID {
		cell["id"] = randomHex(4)
	}
	return marshalPlain(cell)
}

// notebookEdited is the notebook's text after a NotebookEdit call, for the write gates. An edit that
// cannot apply leaves the text as it is: the tool itself reports why.
func notebookEdited(before string, input json.RawMessage) string {
	var in notebookEditInput
	if json.Unmarshal(input, &in) != nil {
		return before
	}
	after, why := applyNotebookEdit([]byte(before), in)
	if !why.Empty() {
		return before
	}
	return string(after)
}
