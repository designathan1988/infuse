package shellparse

import "testing"

func TestMayChangeTree(t *testing.T) {
	for _, cmd := range []string{"ls -la src", "cd src && cat a.txt", "grep -n x a.txt | head"} {
		if MayChangeTree(cmd) {
			t.Errorf("%q reported as changing the tree", cmd)
		}
	}
	for _, cmd := range []string{"mv a b", "ls && rm a", "git status", "echo x > a.txt", "find . -delete"} {
		if !MayChangeTree(cmd) {
			t.Errorf("%q reported as read-only", cmd)
		}
	}
}

func TestDiagnosisReadOnly(t *testing.T) {
	for _, cmd := range []string{"ls -la", "cd src && cat a.txt", "git status", "git diff HEAD"} {
		if !DiagnosisReadOnly(cmd) {
			t.Errorf("%q refused before diagnosis", cmd)
		}
	}
	for _, cmd := range []string{"rm a", "npm test", "cat a > b", "git checkout main"} {
		if DiagnosisReadOnly(cmd) {
			t.Errorf("%q allowed before diagnosis", cmd)
		}
	}
}
