package filter

import (
	"infuse/internal/core"
	"testing"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func newGuard(root string, protected []string) *Guard {
	return NewGuard(root, Policy{Protected: protected})
}

func M(id string, kv ...any) core.Msg { return core.M(id, kv...) }
func msgOf(err error) (core.Msg, bool) { return core.MsgOf(err) }

func FindRealBash() string { return core.FindRealBash() }

func exists(p string) bool { return core.Exists(p) }
