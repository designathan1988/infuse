package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

// Edit: exact replacement of one occurrence.

// ---------------------------------------------------------------- Edit

type editTool struct{}

func (editTool) Category() string { return "write" }
func (editTool) Spec() ToolSpec {
	return ToolSpec{Name: "Edit", Description: "Replaces an exact passage of a file. old_string must occur exactly once (or use replace_all). Read the file before editing it.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"]}`)}
}

func (editTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return tc.SayErr(err), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if !tc.S.wasRead(abs) {
		return tc.Say(M("edit.unread")), true
	}
	content := string(b)
	n := strings.Count(content, in.OldString)
	if in.OldString == "" || n == 0 {
		return tc.Say(M("edit.not_found")), true
	}
	if n > 1 && !in.ReplaceAll {
		return tc.Say(M("edit.ambiguous", "count", n)), true
	}
	tc.S.checkpointBeforeWrite(abs)
	next := strings.Replace(content, in.OldString, in.NewString, map[bool]int{true: -1, false: 1}[in.ReplaceAll])
	if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{tc.Say(M("edit.done", "file", relTo(tc.Root, abs), "count", map[bool]int{true: n, false: 1}[in.ReplaceAll]))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}
