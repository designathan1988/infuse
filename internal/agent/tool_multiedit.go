package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
)

// MultiEdit: atomic edits to one file.

// ---------------------------------------------------------------- MultiEdit

type multiEditTool struct{}

func (multiEditTool) Category() string { return "write" }
func (multiEditTool) Spec() ToolSpec {
	return ToolSpec{Name: "MultiEdit", Description: "Applies several exact replacements to the same file, in order, all at once: either all of them apply or none is written. Each old_string must occur exactly once in the text as it stands after the previous edits (or use replace_all). Read the file first.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old_string","new_string"]}}},"required":["path","edits"]}`)}
}

func (multiEditTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path  string `json:"path"`
		Edits []struct {
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		} `json:"edits"`
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
	if len(in.Edits) == 0 {
		return tc.Say(M("multiedit.none")), true
	}
	content := string(b)
	total := 0
	for i, e := range in.Edits {
		n := strings.Count(content, e.OldString)
		if e.OldString == "" || n == 0 {
			return tc.Say(M("multiedit.not_found", "n", i+1)), true
		}
		if n > 1 && !e.ReplaceAll {
			return tc.Say(M("multiedit.ambiguous", "n", i+1, "count", n)), true
		}
		if e.ReplaceAll {
			content = strings.ReplaceAll(content, e.OldString, e.NewString)
			total += n
		} else {
			content = strings.Replace(content, e.OldString, e.NewString, 1)
			total++
		}
	}
	tc.S.checkpointBeforeWrite(abs)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{tc.Say(M("multiedit.done", "file", relTo(tc.Root, abs), "edits", len(in.Edits), "count", total))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// imageType is the media type of an image the model can see, by extension.
