package filter

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bigTree writes a 5,000-file project and its map.
func bigTree(t *testing.T) string {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/big\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5000; i++ {
		dir := filepath.Join(root, "pkg", fmt.Sprintf("d%03d", i/25))
		if i%25 == 0 {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		body := fmt.Sprintf("package d%03d\n\nimport \"example.com/big/pkg/d000\"\n\n// F%d is one function.\nfunc F%d() int { return d000.F0() + %d }\n", i/25, i, i, i)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("file%d.go", i)), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	projectmap.WriteMap(root, projectmap.BuildMap(root))
	return root
}

func noWatcher(t *testing.T) *int {
	t.Helper()
	spawned := 0
	old := spawnWatcher
	spawnWatcher = func(string) { spawned++ }
	t.Cleanup(func() { spawnWatcher = old })
	return &spawned
}

func timed(t *testing.T, name string, f func()) {
	t.Helper()
	start := time.Now()
	f()
	took := time.Since(start)
	t.Logf("%s: %v", name, took)
	if took > latencyScale*time.Second {
		t.Errorf("%s took %v on 5,000 files: the filter path must stay under a second", name, took)
	}
}

// The filter path on a 5,000-file project, with a dead watcher: a command that wrote one file, a
// plain listing, and an `ls` behind a step that changes the tree. None of them may rebuild the map.
func TestFilterPathStaysUnderASecond(t *testing.T) {
	root := bigTree(t)
	spawned := noWatcher(t)
	rules := &RuleSet{}
	target := filepath.Join(root, "pkg", "d000", "file0.go")
	started := time.Now().UnixMilli()
	if err := os.WriteFile(target, []byte("package d000\n\n// F0 changed.\nfunc F0() int { return 42 }\n\nfunc G() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions := []TransformAction{{Kind: ActHeredocToFile, Path: target, Detail: "written"}}
	timed(t, "command that wrote a file, watcher dead", func() {
		finishShellRun(root, root, "cat > pkg/d000/file0.go", actions, started, rules, nil)
	})
	if *spawned == 0 {
		t.Fatal("the dead watcher was not started again")
	}
	m := projectmap.LoadMap(root)
	if f := m.Files["pkg/d000/file0.go"]; f == nil || !contains(f.Exports, "G") {
		t.Fatalf("the touched file was not applied to the map: %+v", f)
	}
	timed(t, "plain listing", func() {
		TrailerMsgs(root, root, "ls pkg/d001", nil, time.Now().UnixMilli(), rules, nil)
	})
	_ = os.Remove(filepath.Join(root, "pkg", "d001", "file30.go"))
	var out []core.Msg
	timed(t, "ls after a step that changes the tree", func() {
		out = TrailerMsgs(root, root, "rm pkg/d001/file30.go && ls pkg/d001", nil, time.Now().UnixMilli(), rules, nil)
	})
	if text := joinMsgLines(out); strings.Contains(text, "file30.go") || !strings.Contains(text, "file31.go") {
		t.Fatalf("stale listing: %.300s", text)
	}
}

// With no map yet the shell answers from the listed folder and leaves the first build to the watcher.
func TestColdProjectAnswersWithoutABuild(t *testing.T) {
	root := bigTree(t)
	noWatcher(t)
	_ = os.RemoveAll(filepath.Join(root, ".infuse", "map"))
	timed(t, "listing with no map", func() {
		out := joinMsgLines(TrailerMsgs(root, root, "ls pkg/d002", nil, time.Now().UnixMilli(), &RuleSet{}, nil))
		if !strings.Contains(out, "file50.go") {
			t.Fatalf("%.300s", out)
		}
	})
	if _, err := os.Stat(projectmap.MapFile(root)); err == nil {
		t.Fatal("a partial map was stored")
	}
}

// The journal is read from its end: a long history costs nothing to a command.
func TestJournalTailIgnoresALongHistory(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	dir := journal.InfuseDir(root)
	var old strings.Builder
	line := fmt.Sprintf(`{"ts":%d,"kind":"write","path":"old.go"}`+"\n", time.Now().Add(-time.Hour).UnixMilli())
	for old.Len() < 8<<20 {
		old.WriteString(line)
	}
	if err := os.WriteFile(filepath.Join(dir, "journal.jsonl"), []byte(old.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	since := time.Now().UnixMilli()
	journal.Journal(root, journal.JournalEntry{Kind: "write", Path: "new.go"})
	var got []journal.JournalEntry
	timed(t, "journal tail", func() { got = journal.ReadJournalSince(root, since) })
	if len(got) != 1 || got[0].Path != "new.go" {
		t.Fatalf("%+v", got)
	}
	if all := journal.ReadJournalSince(root, 0); len(all) < 1000 {
		t.Fatalf("a walk back past the first window lost entries: %d", len(all))
	}
}

// The shim is named bash, so a watcher it starts must not be read as `bash watch <root>`.
func TestWatcherRoleSurvivesTheShimName(t *testing.T) {
	if _, ok := watcherRole([]string{"watch", "x"}); ok {
		t.Fatal("a user's `bash watch x` was taken for the watcher")
	}
	t.Setenv(watcherRoleEnv, "1")
	if root, ok := watcherRole([]string{"watch", "x"}); !ok || root != "x" {
		t.Fatal("the watcher started by the shim is not recognized")
	}
	if _, ok := watcherRole([]string{"-c", "ls"}); ok {
		t.Fatal("a command was taken for the watcher")
	}
}

// The trailer names only the dependents the work has not reached, carries the oracle's state and the
// remedy of a vice.
func TestTrailerCarriesUntouchedDependentsAndOracleState(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/core.js", "export const a = 1;\n")
	write("src/one.js", "import { a } from './core.js';\nexport const b = a;\n")
	write("src/two.js", "import { a } from './core.js';\nexport const c = a;\n")
	write(".infuse/oracle/checks.json", `{"checks":[{"id":"t","kind":"file","path":"src/core.js","expect":{"exists":true}}]}`)
	projectmap.WriteMap(root, projectmap.BuildMap(root))
	journal.Journal(root, journal.JournalEntry{Kind: "write", Path: "src/one.js"})
	time.Sleep(5 * time.Millisecond) // that write belongs to earlier work, not to this command
	started := time.Now().UnixMilli()
	journal.Journal(root, journal.JournalEntry{Kind: "write", Path: "src/core.js"})
	actions := []TransformAction{{Kind: ActLesson, Vice: "unquoted-heredoc-with-code", Detail: "body has $x"}}
	out := joinMsgLines(TrailerMsgs(root, root, "true", actions, started, &RuleSet{}, nil))
	if !strings.Contains(out, "src/two.js") || strings.Contains(out, "used by 2") || strings.Contains(out, "src/one.js") {
		t.Fatalf("dependents: %s", out)
	}
	if !strings.Contains(out, "oracle: 1 check(s) defined, no verdict yet") {
		t.Fatalf("oracle: %s", out)
	}
	if !strings.Contains(out, "quote the delimiter") {
		t.Fatalf("the vice came without its remedy: %s", out)
	}
}

// What a command cost is recorded, and a slow one is called out with its history.
func TestSlowCommandIsCalledOut(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	journal.RecordCost(root, "npm test", 42000)
	out := joinMsgLines(TrailerMsgs(root, root, "npm test", nil, time.Now().Add(-30*time.Second).UnixMilli(), &RuleSet{}, nil))
	if !strings.Contains(out, "cost: this command took 30") {
		t.Fatalf("%s", out)
	}
	slow := journal.SlowCommands(root, 3)
	if len(slow) != 1 || slow[0].Runs != 2 || slow[0].Command != "npm test" {
		t.Fatalf("%+v", slow)
	}
	fast := joinMsgLines(TrailerMsgs(root, root, "ls -la", nil, time.Now().UnixMilli(), &RuleSet{}, nil))
	if strings.Contains(fast, "cost:") {
		t.Fatalf("%s", fast)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
