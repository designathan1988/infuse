package architecture

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// reGitExec finds a git started directly, whatever the call: the command name in any exec.Command form.
var reGitExec = regexp.MustCompile(`exec\.(Command|CommandContext|LookPath)\([^)\n]*"git(\.exe)?"`)

// TestGitRunsOnlyThroughGitx: a repository's configuration can name programs, so a git started without
// gitx's overrides runs project-controlled code at the user's full rights.
func TestGitRunsOnlyThroughGitx(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || strings.HasPrefix(rel, "internal/platform/gitx/") {
			return
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			t.Fatal(err)
		}
		if loc := reGitExec.FindIndex(b); loc != nil {
			line := 1 + strings.Count(string(b[:loc[0]]), "\n")
			t.Errorf("%s:%d starts git directly; use internal/platform/gitx", rel, line)
		}
	})
}
