package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Write.

// ---------------------------------------------------------------- Write

type writeTool struct{}

func (writeTool) Category() string { return "write" }
func (writeTool) Spec() ToolSpec {
	return ToolSpec{Name: "Write", Description: "Creates or overwrites a file with its complete content. For targeted changes to an existing file, prefer Edit.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)}
}

func (writeTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path, Content string }
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return tc.SayErr(err), true
	}
	tc.S.checkpointBeforeWrite(abs)
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{tc.Say(M("write.done", "file", relTo(tc.Root, abs), "bytes", len(in.Content)))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}
