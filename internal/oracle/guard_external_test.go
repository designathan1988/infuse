package oracle_test

// An external test package: the filter imports the oracle, so a test that drives the guard cannot live
// inside package oracle.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"infuse/internal/filter"
)

func bypassRoot(t *testing.T) (string, *filter.Guard) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	oracle := filepath.Join(root, ".infuse", "oracle", "ready.md")
	_ = os.MkdirAll(filepath.Dir(oracle), 0o755)
	_ = os.WriteFile(oracle, []byte("ORIGINAL\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "secrets"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "secrets", "key.pem"), []byte("K\n"), 0o644)
	return root, filter.NewGuard(root, filter.Policy{Protected: []string{"secrets/**"}})
}

func TestAgentCannotTouchCriteriaOrEvidence(t *testing.T) {
	root, g := bypassRoot(t)
	for _, cmd := range []string{
		"echo '{}' > .infuse/oracle/checks.json && echo x",
		"rm -rf .infuse/evidence",
		"cd .infuse/evidence && rm -f */verdict.json",
	} {
		out, _, err := g.GuardCommand(cmd, root)
		if err == nil && !strings.Contains(out, "/dev/null") {
			t.Fatalf("not refused or neutralized: %q → %q", cmd, out)
		}
	}
}
