package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// B2: a command claim must name the command that actually ran, not a fragment of another one.
func TestCommandClaimNeedsTheExactCommand(t *testing.T) {
	reportProject(t)
	s := newSession(t.TempDir())
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.Messages = []Message{
		{Role: "assistant", Content: []Block{{Type: "tool_use", ID: "1", Name: "Bash", Input: json.RawMessage(`{"command":"npm run build:prod"}`)}}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "1", Content: "ok"}}},
	}
	sub := ReportClaim{Text: "build ok", Kind: "command", Ref: "npm run build"}
	s.verifyClaim(&sub)
	if sub.Status == "confirmado" {
		t.Fatalf("a fragment of another command must not confirm a claim (%q)", sub.Status)
	}
	exact := ReportClaim{Text: "build ok", Kind: "command", Ref: "npm run build:prod"}
	s.verifyClaim(&exact)
	if exact.Status != "confirmado" {
		t.Fatalf("the exact command must confirm the claim, got %q", exact.Status)
	}
}

// B5: a suppression silences the checker only when its reason is on the same line.
func TestSuppressorsNeedAReasonOnTheSameLine(t *testing.T) {
	blocked := []string{
		"// eslint-disable-next-line",
		"//nolint",
		"// @ts-ignore",
		"  } catch (e) {}",
		"# type: ignore",
	}
	for _, line := range blocked {
		if !suppressorWithoutReason(line) {
			t.Fatalf("%q silences a checker without a reason and must be refused", line)
		}
	}
	allowed := []string{
		"// eslint-disable-next-line -- the API returns a string here",
		"//nolint:errcheck",
		"// @ts-expect-error the payload comes from an untyped socket",
		"  } catch (e) { /* the socket is already closed here */ }",
		"# type: ignore[name-defined]",
	}
	for _, line := range allowed {
		if suppressorWithoutReason(line) {
			t.Fatalf("%q carries its reason and must pass", line)
		}
	}
}

// B9: a baseline/threshold/budget file is a verdict container.
func TestBaselineFilesAreRecognized(t *testing.T) {
	for _, f := range []string{"perf/baseline.json", "src/coverage-thresholds.js", ".nycrc.json", "tests/golden/out.txt"} {
		if baselineKind(f) == "" {
			t.Fatalf("%s must be recognized as a verdict container", f)
		}
	}
	if baselineKind("src/app.js") != "" {
		t.Fatal("ordinary source is not a verdict container")
	}
	if _, changed := baselineChange("perf/baseline.json", "a", "b"); !changed {
		t.Fatal("changing a baseline must be caught")
	}
	if _, changed := baselineChange("perf/baseline.json", "a", "a"); changed {
		t.Fatal("a no-op write is not a change")
	}
}

// D4: a turn that leaves a background process must account for it in the Report.
func TestRunningProcessesAreDeclaredBeforeTheTurnEnds(t *testing.T) {
	testHome(t)
	s := newSession(t.TempDir())
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.procs = []*bgProc{{ID: "p1", Command: "npm run dev", State: "running"}}
	s.Messages = []Message{{Role: "assistant", Content: []Block{
		{Type: "tool_use", ID: "R", Name: "Report", Input: json.RawMessage(`{"summary":"done","claims":[{"text":"development server running on p1","kind":"other"}]}`)},
	}}}
	if why := s.missingDeclaredProcs(); !why.Empty() {
		t.Fatalf("a process named in the Report is accounted for: %s", why.Render("en"))
	}
	s.Messages = []Message{{Role: "assistant", Content: []Block{
		{Type: "tool_use", ID: "R", Name: "Report", Input: json.RawMessage(`{"summary":"done","claims":[]}`)},
	}}}
	if why := s.missingDeclaredProcs(); why.Empty() {
		t.Fatal("an unexplained background process must block the turn end")
	}
}

// F6: what a subagent did is not the agent's own evidence in the Report.
func TestSubagentWorkIsNotTheAgentsOwnEvidence(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "a.js"), []byte("export const a = 1;\n"), 0o644)
	s := newSession(root)
	s.Turn = 1
	s.TurnStarts = []int{0}
	s.Checkpoints[1] = []CheckpointFile{{Path: "a.js"}}
	ctx := s.claimContext()

	claim := ReportClaim{Text: "created a.js", Kind: "file", Ref: "a.js"}
	verifyFileClaim(&claim, ctx)
	if claim.Status != "parcial" || !strings.Contains(claim.Evidence.Render("en"), "subagent") {
		t.Fatalf("a file written by a subagent is not confirmed by the agent itself: %q %s", claim.Status, claim.Evidence.Render("en"))
	}

	s.Messages = []Message{{Role: "assistant", Content: []Block{
		{Type: "tool_use", ID: "w", Name: "Write", Input: json.RawMessage(`{"path":"a.js"}`)},
	}}}
	own := ReportClaim{Text: "created a.js", Kind: "file", Ref: "a.js"}
	verifyFileClaim(&own, s.claimContext())
	if own.Status != "confirmado" {
		t.Fatalf("the agent's own write confirms the claim, got %q", own.Status)
	}

	s.Agents = []AgentRun{{ID: "x", Trail: []AgentStep{{Kind: "tool", Tool: "Bash", Input: `{"command":"npm run build"}`}}}}
	if !s.subagentRan("npm run build") {
		t.Fatal("the subagent's command must be found in its trail")
	}
}

func reportProject(t *testing.T) string {
	t.Helper()
	testHome(t)
	return t.TempDir()
}
