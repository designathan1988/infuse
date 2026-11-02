package oracle

import "testing"

func TestLintSpecRefusesHollowAndDuplicatedChecks(t *testing.T) {
	base := OracleCheck{ID: "c1", Kind: "command", Run: "go test ./..."}
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{base}}); err != nil {
		t.Fatal(err)
	}
	cases := map[string]OracleSpec{
		"duplicated id": {Checks: []OracleCheck{base, {ID: "c1", Kind: "file", Path: "README.md"}}},
		"hollow echo":   {Checks: []OracleCheck{{ID: "c2", Kind: "command", Run: "echo done"}}},
		"hollow true":   {Checks: []OracleCheck{{ID: "c3", Kind: "command", Run: "true"}}},
		"duplicated target": {Checks: []OracleCheck{
			{ID: "a", Kind: "command", Run: "go vet ./..."},
			{ID: "b", Kind: "command", Run: "go vet ./..."},
		}},
	}
	for name, spec := range cases {
		if err := LintSpec(spec); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// an echo that observes the world is fine
	if err := LintSpec(OracleSpec{Checks: []OracleCheck{{ID: "x", Kind: "command", Run: `echo $(ls src | wc -l)`}}}); err != nil {
		t.Fatal(err)
	}
}
