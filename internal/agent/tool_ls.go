package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// LS: a directory infused with its map.

// ---------------------------------------------------------------- LS

type lsTool struct{}

func (lsTool) Category() string { return "read" }
func (lsTool) Spec() ToolSpec {
	return ToolSpec{Name: "LS", Description: "Lists a project folder. The answer includes the folder's map: what each file exports, what depends on it, files that are too large.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":[]}`)}
}

func (lsTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	_ = json.Unmarshal(input, &in)
	if in.Path == "" {
		in.Path = "."
	}
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return tc.SayErr(err), true
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return err.Error(), true
	}
	var sb strings.Builder
	hidden := tc.walkFilter()
	for _, e := range entries {
		if hidden(filepath.Join(abs, e.Name()), e) {
			continue // infuse's own records are not listed as part of the project
		}
		if e.IsDir() {
			sb.WriteString(e.Name() + "/\n")
		} else {
			sb.WriteString(e.Name() + "\n")
		}
	}
	rel := relTo(tc.Root, abs)
	if rel == "" {
		rel = "."
	}
	m := LoadOrBuildMap(tc.Root)
	if d := DescribeDir(m, rel); d != "" {
		sb.WriteString("\n" + d + "\n")
	}
	for _, m := range ApplyListRulesMsgs(tc.Root, tc.Rules, rel) {
		sb.WriteString(tc.Say(m) + "\n")
	}
	return tc.clip(sb.String()), false
}
