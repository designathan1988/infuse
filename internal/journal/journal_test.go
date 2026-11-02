package journal

import (
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/data"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every refusal maps to a vice of the catalog, each with its signature and remedy.
func TestGateVicesPointAtTheCatalog(t *testing.T) {
	for id, vice := range GateVices() {
		if sig, remedy := Vice(vice); sig == "" || remedy == "" {
			t.Errorf("%s → %s: not in the catalog", id, vice)
		}
	}
}

// The vices a code path records by name, outside the refusal table.
var namedVices = map[string]string{
	"protected-path-write":       "filter guard",
	"hidden-record-read":         "filter guard",
	"erratic-loop":               "the loop breaker",
	"shortcut-in-test":           "shortcutGuard",
	"test-changed":               "the write sentinel",
	"swallowed-error":            "the write rules",
	"destructive-git":            "the command rules",
	"heredoc-missing-terminator": "the heredoc transform",
	"unquoted-heredoc-with-code": "the heredoc transform",
	"screenshot-missing":         "the screenshot lock",
	"surface-change":             "the screenshot lock",
	"atalho":                     "older ledgers only; no gate records it any more",
}

// No vice is a name in a table: each one has a gate, a detector or a named record.
func TestEveryCatalogViceHasADetector(t *testing.T) {
	var list []viceInfo
	if err := json.Unmarshal(data.Read("vices.json"), &list); err != nil {
		t.Fatal(err)
	}
	mapped := map[string]bool{}
	for _, v := range GateVices() {
		mapped[v] = true
	}
	for name := range namedVices {
		if _, remedy := Vice(name); remedy == "" {
			t.Errorf("%s is recorded by a gate and has no remedy in the catalog", name)
		}
	}
	for _, v := range list {
		if !mapped[v.Vice] && namedVices[v.Vice] == "" {
			t.Errorf("vice %s has no detector", v.Vice)
		}
		if v.Signature == "" || v.Remedy == "" {
			t.Errorf("vice %s has no signature or remedy", v.Vice)
		}
	}
}

// A refusal lands in both records, with the exact vice: in the journal entry and, with its signature
// and remedy, in the ledger.
func TestRefusalRecordsTheExactVice(t *testing.T) {
	root := t.TempDir()
	inner := core.M("arch.cycle", "a", "x")
	Refused(root, JournalEntry{Kind: "gate", Path: "a.go", Detail: "refused"}, core.M("protocol.architecture", "why", inner))
	got := ReadJournalSince(root, 0)
	if len(got) != 1 || got[0].Vice != "import-cycle" || got[0].Path != "a.go" {
		t.Fatalf("%+v", got)
	}
	ledger := core.ReadJSONL(filepath.Join(root, ".infuse", "lessons.jsonl"))
	if len(ledger) != 1 || ledger[0]["vice"] != "import-cycle" || ledger[0]["remedy"] == "" || ledger[0]["signature"] == "" {
		t.Fatalf("%+v", ledger)
	}
	// a wrapper with several causes records each
	both := core.M("vice.structure", "items", []core.Msg{core.M("vice.dead_code"), core.M("vice.feature_spread")})
	if v := VicesOf(both); strings.Join(v, ",") != "dead-code,feature-spread" {
		t.Fatalf("%v", v)
	}
	if v := VicesOf(core.M("agent.no_screenshot", "vice", "surface-change")); len(v) != 1 || v[0] != "surface-change" {
		t.Fatalf("%v", v)
	}
}

// The map, the ledgers and the choice of base are versioned; conversations and blobs are not. An
// old seed is replaced, a file the user edited is not.
func TestGitignoreKeepsArtifactsVersioned(t *testing.T) {
	root := t.TempDir()
	d := InfuseDir(root)
	b, err := os.ReadFile(filepath.Join(d, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, out := range []string{"map/", "lessons.jsonl", "journal.jsonl", "oracle", "evidence"} {
		if strings.Contains(text, out) {
			t.Errorf("%s must be versioned", out)
		}
	}
	for _, in := range []string{"sessions/", "checkpoints/", "tree/", "turnlog/", "shots/", "preview/", "bin/", "watcher.json"} {
		if !strings.Contains(text, in) {
			t.Errorf("%s must stay out of version control", in)
		}
	}
	old := filepath.Join(t.TempDir(), ".infuse")
	_ = os.MkdirAll(old, 0o755)
	_ = os.WriteFile(filepath.Join(old, ".gitignore"), []byte(oldIgnores[0]), 0o644)
	InfuseDir(filepath.Dir(old))
	if b, _ := os.ReadFile(filepath.Join(old, ".gitignore")); strings.Contains(string(b), "map/") {
		t.Fatalf("the old seed stayed: %s", b)
	}
	mine := filepath.Join(t.TempDir(), ".infuse")
	_ = os.MkdirAll(mine, 0o755)
	_ = os.WriteFile(filepath.Join(mine, ".gitignore"), []byte("map/\n"), 0o644)
	InfuseDir(filepath.Dir(mine))
	if b, _ := os.ReadFile(filepath.Join(mine, ".gitignore")); string(b) != "map/\n" {
		t.Fatalf("a file the user edited was replaced: %s", b)
	}
}
