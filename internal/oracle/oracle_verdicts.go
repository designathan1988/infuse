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
		dir := filepath.Join(EvidenceDir(root), entries[i].Name())
		if entries[i].IsDir() && core.ReadJSON(filepath.Join(dir, "verdict.json"), &v) == nil {
			v.SpecVerified = verifySpecCopy(dir, v.SpecHash) && verifyVerdict(v) &&
				verdictBelongs(root, entries[i].Name(), v) && verifyManifest(dir, v.Fingerprint)
			out = append(out, v)
		}
	}
	if out == nil {
		out = []Verdict{}
	}
	return out
}
