package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/base"
	"infuse/internal/core"
	"infuse/internal/journal"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func viceSession(t *testing.T, root string) *Session {
	t.Helper()
	s := newSession(root)
	s.gate.diagnosed = true
	noContract(s)
	s.Mode = "bypass"
	s.gate.planned = true
	return s
}

func lessonVices(root string) []string {
	var out []string
	for _, e := range core.ReadJSONL(filepath.Join(root, ".infuse", "lessons.jsonl")) {
		out = append(out, fmt.Sprint(e["vice"]))
	}
	return out
}

func viceWrite(id, path, content string) Block {
	in, _ := json.Marshal(map[string]string{"path": path, "content": content})
	return Block{Type: "tool_use", ID: id, Name: "Write", Input: in}
}

// A gate that refuses leaves the exact vice in both records, with signature and remedy.
func TestRefusalLeavesItsViceInBothRecords(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := viceSession(t, root)
	s.gate.planned = false
	out, isErr := s.runTool(context.Background(), &RuleSet{}, viceWrite("1", "a.go", "package a\n"))
	if !isErr || !strings.Contains(out, "plan") {
		t.Fatalf("%s", out)
	}
	if got := lessonVices(root); len(got) != 1 || got[0] != "no-plan" {
		t.Fatalf("%v", got)
	}
	entries := journal.ReadJournalSince(root, 0)
	found := false
	for _, e := range entries {
		found = found || e.Kind == "gate" && e.Vice == "no-plan"
	}
	if !found {
		t.Fatalf("%+v", entries)
	}
}

// Shortcuts are no longer lumped together: each records its own vice.
func TestShortcutWarningsRecordTheirExactVice(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := viceSession(t, root)
	s.finishToolOutput(context.Background(), viceWrite("1", "a.js", ""), nil, "", false, []Msg{M("shortcut.empty_catch"), M("shortcut.except_pass")})
	if got := strings.Join(lessonVices(root), ","); got != "empty-catch,except-pass" {
		t.Fatalf("%s", got)
	}
	if ask, _ := detectShortcuts("a.test.js", "", "test.only('x', () => {})"); len(ask) != 1 || ask[0].ID != "shortcut.focus" {
		t.Fatalf("%v", ask)
	}
}

// After a failed test run, taking an export away or switching behavior off is scope cut; the same
// edit with no failure behind it is a refactor.
func TestScopeCutDetector(t *testing.T) {
	testHome(t)
	s := viceSession(t, t.TempDir())
	before := "export function pay() { return charge(); }\nexport function refund() { return 1; }\n"
	removed := "export function pay() { return charge(); }\n"
	if m := s.scopeCut("src/pay.js", before, removed); !m.Empty() {
		t.Fatalf("no failure in the turn: %s", m)
	}
	s.gate.ran = []ranCommand{{cmd: "npm test", ok: false}}
	if m := s.scopeCut("src/pay.js", before, removed); m.ID != "vice.scope_cut" {
		t.Fatalf("%+v", m)
	}
	if m := s.scopeCut("src/pay.js", "const a = 1;\n", "const a = 1;\nconst enabled = false;\n"); m.ID != "vice.scope_cut" {
		t.Fatalf("%+v", m)
	}
	if m := s.scopeCut("src/pay.test.js", before, removed); !m.Empty() {
		t.Fatalf("a test file is judged elsewhere: %s", m)
	}
	renamed := "export function pay() {}\nexport function giveBack() {}\n"
	if m := s.scopeCut("src/pay.js", before, renamed); !m.Empty() {
		t.Fatalf("a rename is not a cut: %s", m)
	}
}

// At the end of the turn the map shows what nothing reaches and what scatters a feature, once.
func TestStructureObligationFindsDeadCodeAndSpread(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("api/billing_routes.ts", "import { charge } from '../domain/billing';\nexport const r = charge;\n")
	write("domain/billing.ts", "export const charge = 1;\n")
	write("legacy/billing_copy.ts", "import { charge } from '../misc/billing_math';\nexport const k = charge;\n")
	write("misc/billing_math.ts", "export const charge = 1;\nexport function neverUsed() {}\n")
	s := viceSession(t, root)
	changed := []CheckpointFile{{Path: "misc/billing_math.ts", New: true}}
	m := s.structureObligation(changed)
	if m.ID != "vice.structure" || !strings.Contains(m.String(), "neverUsed") || !strings.Contains(m.String(), "misc/billing_math.ts (feature billing") {
		t.Fatalf("%s", m)
	}
	vices := strings.Join(journal.VicesOf(m), ",")
	if vices != "dead-code,feature-spread" {
		t.Fatalf("%s", vices)
	}
	if again := s.structureObligation(changed); !again.Empty() {
		t.Fatalf("the audit asks once: %s", again)
	}
}

// The base's layers and its size limit bind the write gate.
func TestBaseLayersAndLimitInTheWriteGate(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if _, err := base.Scaffold(root, "web"); err != nil {
		t.Fatal(err)
	}
	s := viceSession(t, root)
	if m := s.baseViolation("src/core/x.js", true, "import { homePage } from '../pages/home.js';\n"); m.ID != "protocol.layer" {
		t.Fatalf("%+v", m)
	}
	if m := s.baseViolation("src/pages/x.js", true, "import { store } from '../core/store.js';\n"); !m.Empty() {
		t.Fatalf("%s", m)
	}
	s.settings.MaxFileLines = 5000
	if got := s.fileLineLimit(); got != 400 {
		t.Fatalf("the base's limit: %d", got)
	}
	bare := viceSession(t, t.TempDir())
	if got := bare.fileLineLimit(); got != 400 {
		t.Fatalf("default: %d", got)
	}
	bare.settings.MaxFileLines = -1
	if got := bare.fileLineLimit(); got != -1 {
		t.Fatalf("no limit: %d", got)
	}
}

// The agent reads the base's whole contract, the installed versions and the map's levels.
func TestPromptCarriesTheContractAndTheLevels(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if _, err := base.Scaffold(root, "api"); err != nil {
		t.Fatal(err)
	}
	s := viceSession(t, root)
	p := buildSystemPrompt(s, &RuleSet{})
	for _, want := range []string{"may import", "the only writer", "internal/store/persist.go", "How it is done here", "Data and their owners"} {
		if !strings.Contains(p, want) {
			t.Errorf("the prompt omits %q", want)
		}
	}
	if strings.Count(p, "<!-- infuse:begin") != 0 {
		t.Error("a generated block reached the prompt twice")
	}
}

// The write gate does not walk the project for a write with nothing to compare, and a real
// comparison on 5,000 files stays under a second.
func TestSpecialCaseCheckStaysUnderASecond(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5000; i++ {
		dir := filepath.Join(root, "src", fmt.Sprintf("d%03d", i/25))
		if i%25 == 0 {
			_ = os.MkdirAll(dir, 0o755)
		}
		name := fmt.Sprintf("f%d.js", i)
		if i%100 == 0 {
			name = fmt.Sprintf("f%d.test.js", i)
		}
		_ = os.WriteFile(filepath.Join(dir, name), []byte("expect(price).toBe('premium-rate');\nexpect(code).toBe(40412);\n"), 0o644)
	}
	start := time.Now()
	if m := cheatSpecialCase(root, "src/x.js", []string{"const a = 1;", "if (x) { y(); }"}); !m.Empty() {
		t.Fatalf("%s", m)
	}
	plain := time.Since(start)
	start = time.Now()
	m := cheatSpecialCase(root, "src/x.js", []string{"if (plan === 'premium-rate' && code === 40412) return 0;"})
	first := time.Since(start)
	if m.ID != "cheat.special_case" {
		t.Fatalf("the check stopped working: %+v", m)
	}
	start = time.Now()
	cheatSpecialCase(root, "src/x.js", []string{"if (plan === 'premium-rate' && code === 40412) return 0;"})
	second := time.Since(start)
	t.Logf("no literals: %v; first comparison: %v; cached: %v", plain, first, second)
	if plain > 50*time.Millisecond || first > time.Second || second > time.Second {
		t.Fatalf("no literals %v, first %v, cached %v", plain, first, second)
	}
}

// Every gate message of the catalog either maps to a vice or is declared not to be one.
func TestEveryGateMessageMapsToAVice(t *testing.T) {
	gate := regexp.MustCompile(`^(?:cheat|arch|work[.]|sentinel|ban\.refused|protocol|turn\.(?:open|checker|oracle|review$)|gate\.(?:research|redo|no_|report|shell_test)|instr\.(?:no_edit|reference|required|verify|loop)|docs\.(?:cmd|write)|shortcut\.(?:assert|checker|disabled|empty|except|focus|justify|pytest|skip|t_skip|test_true|ts_ignore))`)
	wrappers := map[string]bool{"protocol.architecture": true, "protocol.convention": true, "gate.shell_write": true, "arch.func_lines_hint": true}
	table := journal.GateVices()
	for id := range core.Catalog() {
		if gate.MatchString(id) && !wrappers[id] && table[id] == "" {
			t.Errorf("refusal %s has no vice", id)
		}
	}
}
