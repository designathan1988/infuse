package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Glob: files by pattern, newest first.

// ---------------------------------------------------------------- Glob

type globTool struct{}

func (globTool) Category() string { return "read" }
func (globTool) Spec() ToolSpec {
	return ToolSpec{Name: "Glob", Description: "Finds files by pattern (e.g. src/**/*.ts). Sorted by modification time, most recent first.",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string","description":"Base folder (default: the project root)"}},"required":["pattern"]}`)}
}

func (globTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Pattern, Path string }
	_ = json.Unmarshal(input, &in)
	base, err := tc.walkBase(in.Path)
	if err != nil {
		return tc.SayErr(err), true
	}
	hidden := tc.walkFilter()
	re := globToRegex(strings.TrimPrefix(in.Pattern, "./"))
	type hit struct {
		p string
		t time.Time
	}
	var hits []hit
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if hidden(p, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if re.MatchString(toPosix(rel)) {
			info, _ := d.Info()
			t := time.Time{}
			if info != nil {
				t = info.ModTime()
			}
			hits = append(hits, hit{toPosix(rel), t})
		}
		if len(hits) >= 2000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool { return hits[i].t.After(hits[j].t) })
	var sb strings.Builder
	for _, h := range hits {
		sb.WriteString(h.p + "\n")
	}
	if len(hits) == 0 {
		return tc.Say(M("glob.none")), false
	}
	if m := LoadMap(tc.Root); m != nil { // the same directory context LS gives
		if d := DescribeDir(m, relTo(tc.Root, base)); d != "" {
			sb.WriteString("\n" + d + "\n")
		}
	}
	return tc.clip(sb.String()), false
}
