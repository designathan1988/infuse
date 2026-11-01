package agent

// Lock B9 (agent-failure notes): the files that decide a verification by comparing numbers —
// baselines, thresholds, budgets, golden files, coverage configuration. Changing them changes the
// verdict without changing the work. They are never edited silently: the user decides every time.

import (
	"path/filepath"
	"strings"
)

// baselineKind names what makes a file a verdict container, or "" for an ordinary file.
func baselineKind(rel string) string {
	rel = strings.ToLower(filepath.ToSlash(rel))
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:len(parts)-1] {
		switch {
		case strings.Contains(dir, "baseline"):
			return "baseline"
		case strings.Contains(dir, "golden"):
			return "golden"
		case strings.Contains(dir, "snapshot"):
			return "snapshot"
		case strings.Contains(dir, "threshold"):
			return "threshold"
		case strings.Contains(dir, "budget"):
			return "budget"
		}
	}
	name := filepath.Base(rel)
	switch {
	case strings.Contains(name, "baseline"):
		return "baseline"
	case strings.Contains(name, "threshold"):
		return "threshold"
	case strings.Contains(name, "budget"):
		return "budget"
	case strings.Contains(name, "golden"):
		return "golden"
	case strings.Contains(name, "snapshot") || strings.HasSuffix(name, ".snap"):
		return "snapshot"
	case strings.HasPrefix(name, ".nycrc") || name == "coverage.json" || name == "codecov.yml" || name == "codecov.yaml":
		return "coverage"
	}
	return ""
}

// baselineChange is the verdict for a write to a baseline-like file: empty when nothing changes.
func baselineChange(rel, before, after string) (Msg, bool) {
	kind := baselineKind(rel)
	if kind == "" || before == after {
		return Msg{}, false
	}
	return M("work.baseline", "file", filepath.ToSlash(rel), "kind", kind), true
}
