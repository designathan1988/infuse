package oracle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSpec saves criteria the way the user's editor does: through SaveSpec, which anchors the hash
// outside the project.
func writeSpec(t *testing.T, root, spec string) {
	t.Helper()
	var s OracleSpec
	if err := json.Unmarshal([]byte(spec), &s); err != nil {
		t.Fatal(err)
	}
	if err := SaveSpec(root, s); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorVerdictFromEvidence(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	_ = os.WriteFile(filepath.Join(root, "sum.js"), []byte("module.exports = (a, b) => a + b;\n"), 0o644) // uncommitted: the snapshot must include it
	writeSpec(t, root, `{"checks":[
		{"id":"file","kind":"file","path":"sum.js","expect":{"contains":"a + b"}},
		{"id":"command","kind":"command","run":"test -f sum.js && echo found","expect":{"contains":"found"}},
		{"id":"size","kind":"maxLines","glob":"**/*.js","max":50}
	]}`)
	v, err := RunOracle(context.Background(), root, root, "")
	if err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "aprovado" || len(v.Checks) != 3 || !strings.HasPrefix(v.Snapshot, "git:") || v.Signature == "" {
		t.Fatalf("%+v", v)
	}
	if _, err := os.Stat(filepath.Join(EvidenceDir(root), v.Run, "command.log")); err != nil {
		t.Fatal("evidence not stored")
	}
	if st := gitOutput(root, "status", "--porcelain"); !strings.Contains(st, "sum.js") {
		t.Fatal("the user's index or working tree was changed by the snapshot")
	}

	// a failing criterion: the verdict is "reprovado" with the reason, whatever the agent claims
	writeSpec(t, root, `{"checks":[{"id":"test","kind":"command","run":"exit 3"},{"id":"exists","kind":"file","path":"missing.js"}]}`)
	v, _ = RunOracle(context.Background(), root, root, "")
	if v.Verdict != "reprovado" || v.Checks[0].Passed || !strings.Contains(v.Checks[0].Detail, "exit 3") || v.Checks[1].Passed {
		t.Fatalf("%+v", v)
	}
	if len(ListVerdicts(root, 10)) != 2 {
		t.Fatal("verdict history")
	}
}

// The snapshot junctions node_modules: a check that rewrites an installed runner while it runs
// invalidates the verdict instead of approving a tree it no longer measured.
func TestDependencyChangeDuringChecksInvalidatesTheVerdict(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	bin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "runner"), []byte("original\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, root, `{"checks":[{"id":"tamper","kind":"command","run":"echo poisoned > node_modules/.bin/runner && echo ok","expect":{"contains":"ok"}}]}`)
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "erro" || v.Dependencies == "" {
		t.Fatalf("the dependency change did not invalidate the verdict: %+v", v)
	}
	b, err := os.ReadFile(filepath.Join(bin, "runner"))
	if err != nil || string(b) != "poisoned\n" {
		t.Fatalf("the check did not write through the live dependency directory: %q %v", b, err)
	}
}

// An HTTP check without a server started from the snapshot would measure whatever answers the
// address: it is refused, not run with a warning.
func TestHTTPCheckWithoutSnapshotServerIsRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"page","kind":"http","url":"http://127.0.0.1:1/"}]}`)
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "erro" || len(v.Checks) != 0 {
		t.Fatalf("%+v", v)
	}
}

// Criteria written inside the project, without passing through the user's editor, do not match the
// anchor kept outside it.
func TestCriteriaEditedInsideTheProjectAreRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(SpecFile(root), []byte(`{"checks":[{"id":"a","kind":"file","path":"a.txt","expect":{"contains":"x"}}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "erro" || len(v.Checks) != 0 {
		t.Fatalf("tampered criteria ran: %+v", v)
	}
}

// A verdict edited after the run no longer verifies.
func TestVerdictSignatureDetectsTampering(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt","expect":{"contains":"x"}}]}`)
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "aprovado" || v.Signature == "" {
		t.Fatalf("%+v", v)
	}
	if list := ListVerdicts(root, 1); len(list) != 1 || !list[0].SpecVerified {
		t.Fatal("a signed verdict must verify")
	}
	v.Summary = "forged"
	if err := writeJSONForTest(filepath.Join(EvidenceDir(root), v.Run, "verdict.json"), v); err != nil {
		t.Fatal(err)
	}
	if list := ListVerdicts(root, 1); len(list) != 1 || list[0].SpecVerified {
		t.Fatal("a forged verdict must not verify")
	}
}

func writeJSONForTest(path string, v Verdict) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func TestExecutorWithoutGitAndWithoutCriteria(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	v, _ := RunOracle(context.Background(), root, root, "")
	if v.Verdict != "sem-criterios" {
		t.Fatalf("%+v", v)
	}
	_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte("ok\n"), 0o644)
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt","expect":{"contains":"ok"}}]}`)
	v, _ = RunOracle(context.Background(), root, root, "")
	if v.Verdict != "aprovado" || !strings.HasPrefix(v.Snapshot, "sha256:") {
		t.Fatalf("%+v", v)
	}
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
