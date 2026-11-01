package filter

import (
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runBash(t *testing.T, cmd, cwd string) string {
	t.Helper()
	c := exec.Command(FindRealBash(), "-c", cmd)
	c.Dir = cwd
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatalf("bash failed: %v\n%s", err, out)
	}
	return string(out)
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func TestFindQuotedAndIgnoreInsideQuotes(t *testing.T) {
	hs := FindHeredocs("echo \"a <<b\"; cat <<'EOF'\nline\nEOF\n")
	if len(hs) != 1 || hs[0].Delimiter != "EOF" || !hs[0].Quoted || hs[0].Body != "line\n" || !hs[0].TerminatorFound {
		t.Fatalf("unexpected: %+v", hs)
	}
}

func TestIgnoresHereStrings(t *testing.T) {
	if n := len(FindHeredocs(`cat <<< "x"`)); n != 0 {
		t.Fatalf("expected 0, got %d", n)
	}
}

func TestTolerantTerminator(t *testing.T) {
	hs := FindHeredocs("cat > f <<EOF\r\na\r\n   EOF  \r\necho next")
	if hs[0].Body != "a\n" || !hs[0].TerminatorFound {
		t.Fatalf("unexpected: %+v", hs[0])
	}
}

func TestMissingTerminator(t *testing.T) {
	hs := FindHeredocs("cat > f <<EOF\na\nb")
	if hs[0].Body != "a\nb\n" || hs[0].TerminatorFound {
		t.Fatalf("unexpected: %+v", hs[0])
	}
}

func TestStripTabs(t *testing.T) {
	if hs := FindHeredocs("cat <<-EOF\n\tx\n\tEOF\n"); hs[0].Body != "x\n" {
		t.Fatalf("unexpected: %q", hs[0].Body)
	}
}

func TestTwoHeredocsSameLine(t *testing.T) {
	hs := FindHeredocs("cat <<A > a; cat <<B > b\n1\nA\n2\nB\n")
	if len(hs) != 2 || hs[0].Body != "1\n" || hs[1].Body != "2\n" {
		t.Fatalf("unexpected: %+v", hs)
	}
}

func TestDetectFileWrite(t *testing.T) {
	cases := map[string]*FileWriteIntent{
		"cat > a.txt <<EOF":                 {"a.txt", false},
		"cat >> 'a b.txt' <<'EOF'":          {"a b.txt", true},
		"cat <<EOF > out/x.ts":              {"out/x.ts", false},
		"tee -a log.txt <<EOF":              {"log.txt", true},
		"sudo tee /etc/x <<EOF > /dev/null": {"/etc/x", false},
		"python3 <<EOF":                     nil,
	}
	for in, want := range cases {
		got := DetectFileWrite(in)
		if (got == nil) != (want == nil) || (got != nil && *got != *want) {
			t.Errorf("%q: got %+v want %+v", in, got, want)
		}
	}
}

func TestQuotedFileWriteExactBytes(t *testing.T) {
	dir := t.TempDir()
	cmd := "cat > a.ts <<'EOF'\nconst s = `x${y}` // $(no) \\\" \"q\"\nEOF\necho ok"
	r := TransformCommand(cmd, dir)
	if strings.Contains(r.Command, "<<") {
		t.Fatal("heredoc left in command")
	}
	if got := readFile(t, filepath.Join(dir, "a.ts")); got != "const s = `x${y}` // $(no) \\\" \"q\"\n" {
		t.Fatalf("got %q", got)
	}
	if out := strings.TrimSpace(runBash(t, r.Command, dir)); out != "ok" {
		t.Fatalf("got %q", out)
	}
}

func TestUnquotedCodeIsLiteralAndVice(t *testing.T) {
	dir := t.TempDir()
	r := TransformCommand("cat > a.js <<EOF\nconsole.log(`hi ${name}`)\nEOF", dir)
	if got := readFile(t, filepath.Join(dir, "a.js")); got != "console.log(`hi ${name}`)\n" {
		t.Fatalf("got %q", got)
	}
	found := false
	for _, a := range r.Actions {
		if a.Vice == "unquoted-heredoc-with-code" {
			found = true
		}
	}
	if !found {
		t.Fatal("vice not recorded")
	}
}

func TestUnquotedPlainTextKeepsSameShellExpansion(t *testing.T) {
	dir := t.TempDir()
	r := TransformCommand("N=world; cat > h.txt <<EOF\nhello $N $(echo X) \\$lit \\\"q\\\"\nEOF", dir)
	if strings.Contains(r.Command, "<<") {
		t.Fatal("heredoc left in command")
	}
	runBash(t, r.Command, dir)
	if got := readFile(t, filepath.Join(dir, "h.txt")); got != "hello world X $lit \\\"q\\\"\n" {
		t.Fatalf("got %q", got)
	}
}

func TestHeredocFeedingProgram(t *testing.T) {
	dir := t.TempDir()
	r1 := TransformCommand("python3 <<'PY'\nprint('a'+'b')\nPY", dir)
	if strings.Contains(r1.Command, "<<") || strings.TrimSpace(runBash(t, r1.Command, dir)) != "ab" {
		t.Fatalf("quoted stdin: %q", r1.Command)
	}
	r2 := TransformCommand("V=3; python3 <<PY\nprint($V*2)\nPY", dir)
	if strings.Contains(r2.Command, "<<") || strings.TrimSpace(runBash(t, r2.Command, dir)) != "6" {
		t.Fatalf("unquoted stdin: %q", r2.Command)
	}
}

func TestMissingTerminatorStillWrites(t *testing.T) {
	dir := t.TempDir()
	r := TransformCommand("cat > n.md <<'EOF'\n# t\nbody", dir)
	if !core.Exists(filepath.Join(dir, "n.md")) {
		t.Fatal("file not written")
	}
	found := false
	for _, a := range r.Actions {
		if a.Vice == "heredoc-missing-terminator" {
			found = true
		}
	}
	if !found {
		t.Fatal("vice not recorded")
	}
}

func TestNoHeredocUntouched(t *testing.T) {
	cmd := "ls -la && echo '<<not a heredoc>>'"
	if r := TransformCommand(cmd, t.TempDir()); r.Command != cmd {
		t.Fatalf("changed: %q", r.Command)
	}
}

func TestRulesCommandReplaceAndSay(t *testing.T) {
	root := t.TempDir()
	rep := ": removed"
	set := &RuleSet{Disabled: map[string]bool{}, Rules: []Rule{{ID: "r", When: "command", Match: `git\s+push\s+--force`, Replace: &rep, Say: "no"}}}
	cmd, lines := ApplyCommandRules(root, set, "git push --force origin main")
	if cmd != ": removed origin main" || len(lines) != 1 || lines[0] != "[infuse] no" {
		t.Fatalf("got %q %v", cmd, lines)
	}
}

func TestListedDirectory(t *testing.T) {
	if d, ok := ListedDirectory("cat > x <<EOF\nfoo\nEOF\nls src/tools"); !ok || d != "src/tools" {
		t.Fatalf("got %q %v", d, ok)
	}
	if d, ok := ListedDirectory("cd src && ls -la"); !ok || d != "src" {
		t.Fatalf("got %q %v", d, ok)
	}
}

func TestHeredocUnterminatedQuotedDelimiterDoesNotPanic(t *testing.T) {
	for _, cmd := range []string{`cat <<"000`, `cat <<'EOF`, `cat <<"`, `x <<''`} {
		_ = FindHeredocs(cmd)
		_, _, _ = newGuard(t.TempDir(), nil).GuardCommand(cmd, t.TempDir())
	}
}
