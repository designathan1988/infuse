package oracle

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
)

// Stored verdicts.

func ListVerdicts(root string, n int) []Verdict {
	entries, _ := os.ReadDir(EvidenceDir(root))
	var out []Verdict
	for i := len(entries) - 1; i >= 0 && len(out) < n; i-- {
		var v Verdict
		if entries[i].IsDir() && core.ReadJSON(filepath.Join(EvidenceDir(root), entries[i].Name(), "verdict.json"), &v) == nil {
			out = append(out, v)
		}
	}
	if out == nil {
		out = []Verdict{}
	}
	return out
}
