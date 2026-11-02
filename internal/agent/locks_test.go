package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func lockProject(t *testing.T, files map[string]string) string {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	for p, c := range files {
		abs := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(abs), 0o755)
		_ = os.WriteFile(abs, []byte(c), 0o644)
	}
	return root
}

func wantMsg(t *testing.T, got Msg, id string) {
	t.Helper()
	if got.ID != id {
		t.Fatalf("want %q, got %q (%s)", id, got.ID, got.String())
	}
}

func TestSwallowedTestExitIsRefused(t *testing.T) {
	wantMsg(t, swallowedTestCommand("npm test || true"), "cheat.swallowed_exit")
	wantMsg(t, swallowedTestCommand("cd app && go test ./... ; exit 0"), "cheat.swallowed_exit")
	wantMsg(t, swallowedTestCommand("go test ./..."), "")
	wantMsg(t, swallowedTestCommand("grep foo x || true"), "") // not a test run
}

func TestTestGamingInCodeIsRefused(t *testing.T) {
	root := lockProject(t, map[string]string{
		"test/price.test.js": "test('discount', () => { expect(price('christmas-coupon', 1999)).toBe(1599); });\n",
		"src/a.js":           "export const a = 1;\n",
	})
	// the tests' expected values used as conditions in production code
	special := "export function price(c, v) {\n  if (c === 'christmas-coupon' && v === 1999) return 1599;\n  return v;\n}\n"
	wantMsg(t, cheatWriteCheck(root, "src/price.js", "", special), "cheat.special_case")
	wantMsg(t, cheatWriteCheck(root, "src/price.js", "", "export function price(c, v) {\n  return c ? v * 0.8 : v;\n}\n"), "")
	// test infrastructure
	wantMsg(t, cheatWriteCheck(root, "conftest.py", "", "import pytest\n"), "cheat.test_infra")
	wantMsg(t, cheatWriteCheck(root, "vitest.config.ts", "export default {}\n", "export default { test: { passWithNoTests: true } }\n"), "cheat.test_infra")
	// equality and clocks
	wantMsg(t, cheatWriteCheck(root, "src/money.py", "", "class Money:\n    def __eq__(self, other):\n        return True\n"), "cheat.always_equal")
	wantMsg(t, cheatWriteCheck(root, "src/bench.js", "", "performance.now = () => 0;\n"), "cheat.clock_patch")
	// unfinished markers
	wantMsg(t, cheatWriteCheck(root, "src/a.js", "export const a = 1;\n", "export const a = 1; // TODO: real value\n"), "cheat.markers")
	// a function hollowed into a stub
	full := "export function total(items) {\n  let s = 0;\n  for (const i of items) {\n    s += i.price * i.qty;\n  }\n  return s;\n}\n"
	wantMsg(t, cheatWriteCheck(root, "src/total.js", full, "export function total(items) {\n  return 0;\n}\n"), "cheat.stub")
	wantMsg(t, cheatWriteCheck(root, "src/total.js", full, strings.Replace(full, "i.qty", "i.quantity", 1)), "")
	// prose is not a test runner: examples and TODO lists in docs are not code fraud
	wantMsg(t, cheatWriteCheck(root, "README.md", "", "```js\nprocess.exit(0);\n```\n"), "")
	wantMsg(t, cheatWriteCheck(root, "docs/testing.md", "", "// eslint-disable-next-line\nTODO: write more tests\n"), "")
}

func TestChangedScreenMustBeLookedAt(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.noteUIChange("src/components/Card.tsx")
	wantMsg(t, s.missingScreenshot(), "agent.no_screenshot")
	s.noteScreenshot()
	wantMsg(t, s.missingScreenshot(), "")
	s.noteUIChange("src/styles/app.css") // a change after the photo needs a new one
	wantMsg(t, s.missingScreenshot(), "agent.no_screenshot")
	s.resetTurnLocks()
	s.noteUIChange("src/util/math.ts") // not a screen
	wantMsg(t, s.missingScreenshot(), "")
}

func TestLargeRemovalNeedsTheDiagnosisOrPlan(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	before := strings.Repeat("line of code\n", 40)
	wantMsg(t, s.largeRemoval("src/feature.js", before, "x\n"), "work.large_removal")
	s.Tasks = []TaskItem{{ID: "1", Text: "remove the old src/feature.js, replaced by the new module", Status: "in_progress", Proof: "build"}}
	wantMsg(t, s.largeRemoval("src/feature.js", before, "x\n"), "")
	wantMsg(t, s.largeRemoval("src/other.js", before, before+"y\n"), "")
}

func TestCitedAddressesMustHaveBeenFetched(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.noteFetched("<external-content source=\"https://docs.example.org/api/v2#auth\">")
	got := s.unfetchedCitations("According to https://docs.example.org/api/v2 and https://invented.dev/guide, ...")
	if len(got) != 1 || got[0] != "https://invented.dev/guide," && got[0] != "https://invented.dev/guide" {
		t.Fatalf("%v", got)
	}
	if len(s.unfetchedCitations("see http://localhost:3000/test")) != 0 {
		t.Fatal("a local address needs no fetching")
	}
}

func TestInventedPackagesAreRefused(t *testing.T) {
	testHome(t)
	// The gate reads the local cache; the registry is asked in the background, never in the gate.
	rememberPackage(pkgRef{name: "left-pad", registry: "npm"}, true, time.Now().AddDate(-8, 0, 0))
	rememberPackage(pkgRef{name: "reactt-dom-utils", registry: "npm"}, false, time.Time{})
	rememberPackage(pkgRef{name: "huggingface-cli-fast", registry: "pypi"}, true, time.Now().AddDate(0, 0, -3))
	refs := packagesInCommand("npm install --save left-pad@1.3.0 reactt-dom-utils && pip install huggingface-cli-fast==0.1 -U")
	if len(refs) != 3 {
		t.Fatalf("%+v", refs)
	}
	wantMsg(t, unknownPackages(context.Background(), "npm i left-pad"), "")
	wantMsg(t, unknownPackages(context.Background(), "npm i left-pad reactt-dom-utils"), "pkg.missing")
	wantMsg(t, unknownPackages(context.Background(), "pip install huggingface-cli-fast"), "pkg.fresh")
	wantMsg(t, unknownPackages(context.Background(), "npm install"), "") // from the lockfile
}

// A package the cache does not know is looked up off the gate path and remembered for the next run.
func TestPackageLookupRunsOffTheGatePath(t *testing.T) {
	testHome(t)
	saved := registryLookup
	t.Cleanup(func() { registryLookup = saved })
	looked := make(chan string, 1)
	registryLookup = func(ctx context.Context, p pkgRef) (bool, time.Time, error) {
		looked <- p.name
		return false, time.Time{}, nil
	}
	wantMsg(t, unknownPackages(context.Background(), "npm i mystery-pkg-xyz"), "") // not blocked on the network
	select {
	case name := <-looked:
		if name != "mystery-pkg-xyz" {
			t.Fatalf("looked up %q", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the background lookup never ran")
	}
}

func TestUserProhibitionsAreCompiledWithQuotesAndEnforced(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Turn = 1
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "Investigate the GPU. Do not use subagents, do not run npm run budget, and do not touch src/legacy. No commits for now."}}}}
	p := &scriptProvider{steps: []func(ModelRequest) (ModelResult, error){func(ModelRequest) (ModelResult, error) {
		return ModelResult{Content: []Block{{Type: "text", Text: `{"bans":[
			{"kind":"tool","value":"Agent","quote":"Do not use subagents"},
			{"kind":"command","value":"npm run budget","quote":"do not run npm run budget"},
			{"kind":"path","value":"src/legacy","quote":"do not touch src/legacy"},
			{"kind":"action","value":"commit","quote":"No commits for now"},
			{"kind":"tool","value":"Browser","quote":"never open the browser"}]}`}}, StopReason: "end_turn"}, nil
	}}}
	s.compileBans(context.Background(), p)
	bans := s.Bans()
	if len(bans) != 4 { // the Browser one was never written by the user
		t.Fatalf("%+v", bans)
	}
	in := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	wantMsg(t, s.banCheck("Agent", in(map[string]string{"kind": "explore"})), "ban.refused")
	wantMsg(t, s.banCheck("Bash", in(map[string]string{"command": "cd app && npm run budget -- --view street"})), "ban.refused")
	wantMsg(t, s.banCheck("Bash", in(map[string]string{"command": "npm run build"})), "")
	wantMsg(t, s.banCheck("Write", in(map[string]string{"path": "src/legacy/x.js"})), "ban.refused")
	wantMsg(t, s.banCheck("Write", in(map[string]string{"path": "src/new/x.js"})), "")
	wantMsg(t, s.banCheck("Bash", in(map[string]string{"command": "git add -A && git commit -m x"})), "ban.refused")
	// the user lifts one on screen; a recompilation keeps it lifted
	s.liftBan(0, true)
	wantMsg(t, s.banCheck("Agent", in(map[string]string{"kind": "explore"})), "")
	s.compileBans(context.Background(), p)
	wantMsg(t, s.banCheck("Agent", in(map[string]string{"kind": "explore"})), "")
}
