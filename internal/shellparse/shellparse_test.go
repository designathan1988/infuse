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

func TestDestructiveGit(t *testing.T) {
	for _, cmd := range []string{"git clean -fdx", "git -C . clean -fdx", "git reset --hard HEAD~1", "git checkout -- .", "git restore src/a.go", "git branch -D main", "git push --force origin main", "git rm file"} {
		if !DestructiveGit(cmd) {
			t.Errorf("%q was not recognized as destructive", cmd)
		}
	}
	for _, cmd := range []string{"git status", "git diff HEAD", "git log --oneline", "git checkout main", "npm test"} {
		if DestructiveGit(cmd) {
			t.Errorf("%q was falsely marked destructive", cmd)
		}
	}
}

func TestDiagnosisReadOnly(t *testing.T) {
	for _, cmd := range []string{
		"ls -la", "cd src && cat a.txt", "git status", "git diff HEAD", "git log --oneline",
		"node --version; npm --version; python --version", "go version", "command -v node",
		"sort file.txt", "find . -name '*.go'", "uname -a", "jq . package.json",
	} {
		if !DiagnosisReadOnly(cmd) {
			t.Errorf("%q refused before diagnosis", cmd)
		}
	}
	for _, cmd := range []string{
		"rm a", "npm test", "cat a > b", "git checkout main", "git add .", "git commit -m x",
		"git stash", "git switch main", "date -s 12:00", "hostname new-name",
		"node -e \"require('fs').writeFileSync('x','y')\"", "python -c 'open(\"x\",\"w\")'",
		"sort -o out file.txt", "find . -delete", "find . -fprint list.txt",
	} {
		if DiagnosisReadOnly(cmd) {
			t.Errorf("%q allowed before diagnosis", cmd)
		}
	}
}
