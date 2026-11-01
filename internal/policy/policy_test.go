package policy

import (
	"os"
	"path/filepath"
	"testing"

	"infuse/internal/config"
	"infuse/internal/store"
)

func testHome(t *testing.T) {
	t.Helper()
	t.Setenv("INFUSE_HOME", t.TempDir())
}

func TestEffectiveMergesDefaultsGlobalAndProject(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	p, err := Effective(root)
	if err != nil || p.Method.AskBudget == nil || *p.Method.AskBudget != 6 || p.Done.Review == nil || !*p.Done.Review {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	if err := Save("", true, Policy{Method: Method{AskBudget: intp(9)}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, false, Policy{Method: Method{Plan: boolp(false)}, Done: DonePolicy{Review: boolp(false)}}); err != nil {
		t.Fatal(err)
	}
	p, err = Effective(root)
	if err != nil {
		t.Fatal(err)
	}
	if *p.Method.AskBudget != 9 || *p.Method.Plan || *p.Done.Review || p.Method.Diagnosis == nil || !*p.Method.Diagnosis {
		t.Fatalf("merge: %+v", p)
	}
	raw, err := readFile(File(root, false))
	if err != nil || raw == nil || raw.SchemaVersion != store.Version {
		t.Fatalf("schema version: %+v %v", raw, err)
	}
}

func TestApplySettingsOnlyUsesExplicitValues(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	s := config.DefaultSettings()
	global, project, err := LoadScopes(root)
	if err != nil {
		t.Fatal(err)
	}
	ApplySettings(&s, global, project) // no files: settings untouched
	if s.MaxFileLines != 0 || s.RequireReview != nil {
		t.Fatalf("defaults leaked into settings: %+v", s)
	}
	if err := Save(root, false, Policy{
		Permissions:  Permissions{Tools: map[string]string{"Bash": "deny"}, ProtectedPaths: []string{"secrets/**"}},
		Method:       Method{AskBudget: intp(3), Plan: boolp(false)},
		Architecture: Architecture{MaxFileLines: intp(700), Enforce: boolp(false)},
		Done:         DonePolicy{RequireContract: boolp(false)},
	}); err != nil {
		t.Fatal(err)
	}
	global, project, _ = LoadScopes(root)
	ApplySettings(&s, global, project)
	if s.AskBudget != 3 || s.RequirePlan == nil || *s.RequirePlan || s.Permissions["Bash"] != "deny" || s.MaxFileLines != 700 || s.EnforceArchitecture == nil || *s.EnforceArchitecture || s.RequireContract == nil || *s.RequireContract {
		t.Fatalf("explicit values not applied: %+v", s)
	}
	if len(s.ProtectedPaths) == 0 || s.ProtectedPaths[len(s.ProtectedPaths)-1] != "secrets/**" {
		t.Fatalf("protected paths: %v", s.ProtectedPaths)
	}
}

func TestProhibitionsUnionAndValidation(t *testing.T) {
	a := &Policy{Prohibitions: Prohibitions{Commands: []string{"npm run deploy"}, Paths: []string{"src/legacy"}}}
	b := &Policy{Prohibitions: Prohibitions{Commands: []string{"npm run deploy", "rm -rf"}, Actions: []string{"push"}}}
	p := ProhibitionsFrom(a, b)
	if len(p.Commands) != 2 || len(p.Paths) != 1 || len(p.Actions) != 1 {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []Policy{
		{Permissions: Permissions{Mode: "banana"}},
		{Permissions: Permissions{Tools: map[string]string{"Bash": "yes"}}},
		{Method: Method{AskBudget: intp(0)}},
		{Method: Method{AskBudget: intp(21)}},
		{Architecture: Architecture{MaxFileLines: intp(5)}},
		{Prohibitions: Prohibitions{Actions: []string{"merge"}}},
	} {
		if Validate(bad) == nil {
			t.Fatalf("accepted invalid policy: %+v", bad)
		}
	}
}

func TestCorruptPolicyIsAnError(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".infuse"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(root, false), []byte(`{"method":`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Effective(root); err == nil {
		t.Fatal("a corrupt policy was silently accepted")
	}
}
