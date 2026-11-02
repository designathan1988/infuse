package filter

import (
	"infuse/internal/core"
	"infuse/internal/sandbox"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The transform writes on the host: it must land where the shell would, and where the guard judged.
func TestTransformFollowsTheShellsDirectory(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{".infuse", "src"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	TransformCommand("cd ../src\ncat > note.txt <<'E'\nx\nE", filepath.Join(root, ".infuse"))
	if !core.Exists(filepath.Join(root, "src", "note.txt")) {
		t.Fatal("the heredoc did not land where the shell is")
	}
	if core.Exists(filepath.Join(root, ".infuse", "note.txt")) {
		t.Fatal("the heredoc landed in the starting directory")
	}
	targets := HeredocTargets("cd ../src\ncat > note.txt <<'E'\nx\nE", filepath.Join(root, ".infuse"))
	if len(targets) != 1 || targets[0].Abs != filepath.Join(root, "src", "note.txt") || targets[0].Content != "x\n" {
		t.Fatalf("%+v", targets)
	}
}

// The audit's case: judged as src/oracle/checks.json, it was written to .infuse/oracle/checks.json.
func TestGuardAndTransformAgreeOnTheTarget(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := "cd ../src\ncat > oracle/checks.json <<'E'\n{}\nE"
	cwd := filepath.Join(root, ".infuse")
	if _, _, err := newGuard(root, nil).GuardCommand(cmd, cwd); err != nil {
		t.Fatalf("src/oracle is a fine place to write: %v", err)
	}
	TransformCommand(cmd, cwd)
	if core.Exists(filepath.Join(root, ".infuse", "oracle", "checks.json")) {
		t.Fatal("the oracle was written through the transform")
	}
	if !core.Exists(filepath.Join(root, "src", "oracle", "checks.json")) {
		t.Fatal("the file was not written where it was judged")
	}
}

func TestHeredocOutsideTheProjectIsRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	outside := t.TempDir()
	g := newGuard(root, nil)
	refused(t, g, root,
		"cat > ../x.txt <<'E'\nb\nE",
		"cat > "+filepath.ToSlash(filepath.Join(outside, "x.txt"))+" <<'E'\nb\nE",
		"cd .. && cat > x.txt <<'E'\nb\nE",
		"cd ..\ncat > x.txt <<'E'\nb\nE",
		"tee ../x.txt <<'E'\nb\nE",
	)
	allowed(t, g, root, "cat > src/x.txt <<'E'\nb\nE", "cat > /dev/null <<'E'\nb\nE", "cat > \"$HOME/x\" <<'E'\nb\nE", "python3 <<'E'\nprint(1)\nE")
	if err := os.Symlink(outside, filepath.Join(root, "out")); err == nil {
		refused(t, g, root, "cat > out/x.txt <<'E'\nb\nE")
	}
}

// A worktree session writes in the worktree, as Write does; the main project is outside it.
func TestHeredocStaysInTheWorktree(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	work := filepath.Join(root, ".infuse", "worktrees", "w")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	g := newGuard(root, nil).In(work)
	refused(t, g, work, "cat > ../../../src/x.txt <<'E'\nb\nE")
	allowed(t, g, work, "cat > src/x.txt <<'E'\nb\nE")
}

// What the shell must expand, and the null device, are not host writes.
func TestTransformLeavesExpansionsToTheShell(t *testing.T) {
	root := t.TempDir()
	r := TransformCommand("cat > \"$OUT/x.txt\" <<'E'\nb\nE", root)
	if !strings.Contains(r.Command, "$OUT") || core.Exists(filepath.Join(root, "$OUT")) {
		t.Fatalf("expansion written literally: %q", r.Command)
	}
	if got := HeredocTargets("cat > /dev/null <<'E'\nb\nE", root); len(got) != 0 {
		t.Fatalf("%+v", got)
	}
	TransformCommand("cat > /dev/null <<'E'\nb\nE", root)
	if core.Exists(filepath.Join(root, "dev")) {
		t.Fatal("the null device became a file in the project")
	}
}

// Document text is not a command, even when it looks like one.
func TestHeredocBodyIsDocumentText(t *testing.T) {
	root := t.TempDir()
	cmd := "cat > doc.md <<'E'\nrun: cat > /etc/passwd <<'F'\ndon't\nF\nE\ncat > b.txt <<'G'\nb\nG"
	hs := FindHeredocs(cmd)
	if len(hs) != 2 || hs[0].Delimiter != "E" || hs[1].Delimiter != "G" {
		t.Fatalf("%+v", hs)
	}
	testHome(t)
	allowed(t, newGuard(root, nil), root, cmd)
}

func TestReguardJudgesWhatARuleRewrote(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	rep := "rm " + oraclePath
	set := &RuleSet{Disabled: map[string]bool{}, Rules: []Rule{{ID: "r", When: "command", Match: `^safe$`, Replace: &rep}}}
	g := newGuard(root, nil)
	cmd, _ := ApplyCommandRules(root, set, "safe")
	if _, _, err := g.Reguard("safe", cmd, root); err == nil {
		t.Fatal("a rule rewrote the command into a protected write and nothing looked")
	}
	if out, notes, err := g.Reguard("ls", "ls", root); err != nil || out != "ls" || notes != nil {
		t.Fatal("an unchanged command needs no second look")
	}
}

func TestInfuseFileStaysInTheProject(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, ".infuse", "sessions", "s.json")
	if err := os.WriteFile(record, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.md"), []byte("project note"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{outside, "../" + filepath.Base(filepath.Dir(outside)) + "/secret.txt", ".infuse/sessions/s.json", record} {
		if got := strings.Join(Rule{ID: "r", InfuseFile: p}.Expand(root), " "); strings.Contains(got, "TOPSECRET") {
			t.Errorf("%s was read: %s", p, got)
		}
	}
	if got := strings.Join(Rule{ID: "r", InfuseFile: "note.md"}.Expand(root), " "); !strings.Contains(got, "project note") {
		t.Fatalf("a project file stopped working: %s", got)
	}
}

// bash x.sh and bash -s cannot be guarded: they run in the sandbox of the project, through this binary.
func TestShimContainsWhatItCannotGuard(t *testing.T) {
	testHome(t)
	t.Setenv("INFUSE_NO_SANDBOX", "")
	if !sandbox.Available() {
		t.Skip("no sandbox on this platform")
	}
	root := t.TempDir()
	for _, args := range [][]string{{"x.sh"}, {"-s"}, {"-c", "echo hi"}, {"--login", "-c", "echo hi"}} {
		argv := containedArgv(root, "bash", args)
		if len(argv) < 3 || argv[1] != sandbox.Role {
			t.Fatalf("%v runs uncontained: %v", args, argv)
		}
		if argv[len(argv)-len(args)-1] != "bash" {
			t.Fatalf("%v lost the real bash: %v", args, argv)
		}
	}
}

// A shell inside the sandbox stays as it is: the wrapper could not label anything from there.
func TestShimDoesNotWrapTwice(t *testing.T) {
	testHome(t)
	t.Setenv("INFUSE_NO_SANDBOX", "")
	t.Setenv("INFUSE_CONTAINED", "1")
	if argv := containedArgv(t.TempDir(), "bash", []string{"x.sh"}); len(argv) != 2 || argv[0] != "bash" {
		t.Fatalf("wrapped again: %v", argv)
	}
}

// Containing a home folder or a drive would open it, not close it.
func TestShimDoesNotContainTheMachine(t *testing.T) {
	testHome(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if containable(home) {
		t.Fatal("the home folder is no project")
	}
	drive := string(filepath.Separator)
	if runtime.GOOS == "windows" {
		drive = filepath.VolumeName(os.TempDir()) + `\`
	}
	if containable(drive) {
		t.Fatal("a drive is no project")
	}
	if !containable(t.TempDir()) {
		t.Fatal("an ordinary folder must be contained")
	}
}

func TestShimRefusesToRunItself(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !isSelf(self) || isSelf(os.TempDir()) {
		t.Fatal("isSelf")
	}
	if code := passthrough(t.TempDir(), self, []string{"-c", "true"}); code != 127 {
		t.Fatalf("code %d", code)
	}
}
