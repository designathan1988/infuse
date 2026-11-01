package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"infuse/internal/policy"
)

func policyBool(v bool) *bool { return &v }
func policyInt(v int) *int    { return &v }

// The project policy changes the settings a session runs with and becomes enforceable prohibitions.
func TestPolicyAppliesToTheSession(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := policy.Save(root, false, policy.Policy{
		Permissions: policy.Permissions{Tools: map[string]string{"Bash": "deny"}, ProtectedPaths: []string{"secrets/**"}},
		Prohibitions: policy.Prohibitions{
			Commands: []string{"npm run deploy"},
			Paths:    []string{"src/legacy"},
			Actions:  []string{"push"},
		},
		Method: policy.Method{AskBudget: policyInt(3), Plan: policyBool(false)},
		Done:   policy.DonePolicy{RequireContract: policyBool(false)},
	}); err != nil {
		t.Fatal(err)
	}
	s := newSession(root)
	s.refreshPolicy()
	if s.askBudget() != 3 {
		t.Fatalf("ask budget %d", s.askBudget())
	}
	if s.requirePlan() {
		t.Fatal("plan requirement was not disabled by the policy")
	}
	if s.settings.Permissions["Bash"] != "deny" {
		t.Fatalf("tool permission %q", s.settings.Permissions["Bash"])
	}
	if why := s.banViolationPath("src/legacy/x.js"); why.ID != "ban.policy" {
		t.Fatalf("policy path prohibition: %q", why.Render("en"))
	}
	why := s.banCheck("Bash", json.RawMessage(`{"command":"git push origin master"}`))
	if why.ID != "ban.policy" {
		t.Fatalf("policy action prohibition: %q", why.Render("en"))
	}
	if why := s.banCheck("Bash", json.RawMessage(`{"command":"npm run deploy"}`)); why.ID != "ban.policy" {
		t.Fatalf("policy command prohibition: %q", why.Render("en"))
	}
}

// A corrupt policy is fail-closed: the session records the error and no turn may run.
func TestCorruptPolicyBlocksTheSession(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	if err := os.MkdirAll(filepath.Join(root, ".infuse"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".infuse", "policy.json"), []byte(`{`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.refreshPolicy()
	if s.policyErr == nil {
		t.Fatal("a corrupt policy was accepted")
	}
}
