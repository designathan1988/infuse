package filter

import (
	"infuse/internal/journal"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func refused(t *testing.T, g *Guard, root string, cmds ...string) {
	t.Helper()
	for _, c := range cmds {
		if _, _, err := g.GuardCommand(c, root); err == nil {
			t.Errorf("not refused: %q", c)
		}
	}
}

func allowed(t *testing.T, g *Guard, root string, cmds ...string) {
	t.Helper()
	for _, c := range cmds {
		if _, _, err := g.GuardCommand(c, root); err != nil {
			t.Errorf("refused: %q: %v", c, err)
		}
	}
}

const oraclePath = ".infuse/oracle/ready.md"

func TestGuardReadsThePrefixesOfACommand(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	refused(t, g, root,
		"X=1 rm "+oraclePath,
		"A=1 B=2 rm "+oraclePath,
		"{ rm "+oraclePath+"; }",
		"(rm "+oraclePath+")",
		"( cd .infuse/oracle && rm ready.md )",
		"if true; then rm "+oraclePath+"; fi",
		"for f in a; do rm "+oraclePath+"; done",
		"timeout 5 rm "+oraclePath,
		"timeout -k 5 10 rm "+oraclePath,
		"timeout -s KILL 5 rm "+oraclePath,
		"sudo -u bob rm "+oraclePath,
		"nice -n 5 rm "+oraclePath,
		"env A=1 rm "+oraclePath,
		"f() { rm "+oraclePath+"; }; f",
		"echo $(rm "+oraclePath+")",
		"echo \"$(rm "+oraclePath+")\"",
		"echo `rm "+oraclePath+"`",
		"rm \\\n"+oraclePath,
		"rm \\\r\n"+oraclePath,
		"# it's a comment\nrm "+oraclePath,
		"case x in a) rm "+oraclePath+";; esac",
	)
}

func TestGuardNeutralizesEveryRedirectionForm(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	for _, c := range []string{
		"echo x &> " + oraclePath,
		"echo x &>> " + oraclePath,
		"echo x >& " + oraclePath,
		"echo x 12> " + oraclePath,
		"echo x >a>" + oraclePath,
		"(echo x > " + oraclePath + ")",
	} {
		out, notes, err := g.GuardCommand(c, root)
		if err != nil || len(notes) == 0 || strings.Contains(out, oraclePath) {
			t.Errorf("%q: out %q notes %v err %v", c, out, notes, err)
		}
	}
	for _, c := range []string{"make 2>&1", "make 2>&1 | tee build.log", "x >&2"} {
		if out, notes, err := g.GuardCommand(c, root); err != nil || len(notes) != 0 || out != c {
			t.Errorf("%q changed: %q %v %v", c, out, notes, err)
		}
	}
}

func TestGuardRefusesWhatHoldsAProtectedPath(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, []string{"secrets/**"})
	sub := filepath.Join(root, "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	refused(t, g, root,
		"rm -rf .", "rm -rf ./", "rm -rf ..", "rm -rf "+filepath.ToSlash(root), "rmdir .infuse",
		"rm -rf .infuse", "mv .infuse x", "mv .git y", "rm -rf secrets", "del /s /q .infuse", "rd /s /q .infuse",
		"cmd /c rd /s /q .infuse", "Remove-Item -Recurse .infuse",
		"cp -r evil .infuse", "cp -r evil .infuse/", "cp -r evil/.infuse .", "mv evil .infuse", "robocopy evil .infuse /E",
		"rsync -a evil/ .infuse", "git rm -r .", "git mv .infuse x",
		"cd src && rm -rf ..", "cd src && rm -rf ../.infuse",
	)
	refused(t, g, sub, "rm -rf ..")
	allowed(t, g, root, "rm -rf node_modules dist", "rm -f build/x.o", "cp a.txt .", "cp -r src backup", "mv a.txt src/", "cp -r . /tmp/x", "mkdir -p src/new")
}

func TestGuardRefusesGitCleanOfTheTree(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	refused(t, g, root, "git clean -fdx", "git clean -f", "git clean -xdf .", "git clean -fdX -- .", "git clean -fd -e x", "git -C . clean -fdx", "cd .infuse; git clean -fd .")
	allowed(t, g, root, "git clean -n", "git clean -nfdx", "git clean --dry-run -d", "git clean -fd build/", "git status", "git log -5")
}

func TestGuardKeepsGitControlFiles(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	refused(t, g, root,
		"cp evil .git/hooks/pre-commit", "rm .git/config", "sed -i s/a/b/ .git/config", "git config core.hooksPath evil",
		"git config --local core.fsmonitor evil", "git -C . config core.sshCommand evil", "git config --unset user.name",
	)
	allowed(t, g, root, "git config user.name", "git config --get user.name", "git config --list", "git config --global user.name me", "git status")
	_, _, err := g.GuardCommand("cp evil .git/hooks/pre-commit", root)
	if m, _ := msgOf(err); m.ID != "guard.git_control" {
		t.Fatalf("not the clear refusal: %v", err)
	}
	for _, p := range []string{".git/config", ".git/hooks", ".git/hooks/pre-commit", ".GIT/CONFIG."} {
		if os.PathSeparator == '/' && strings.ContainsAny(p, "GC") {
			continue
		}
		if _, ok := g.Protects(root, p); !ok {
			t.Errorf("%s is not protected", p)
		}
	}
	if _, ok := g.Protects(root, ".git/HEAD"); ok {
		t.Fatal("the guard must not cover the rest of .git")
	}
}

func TestGuardReadsDownloadAndPowerShellFlags(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	enc := "powershell -e " + encodePS("Set-Content .infuse/oracle/x y")
	refused(t, g, root,
		"curl -o"+oraclePath+" http://x", "curl -sSo "+oraclePath+" http://x", "curl -sSo"+oraclePath+" http://x",
		"curl --output="+oraclePath+" http://x", "wget -O"+oraclePath+" http://x", "wget --output-document="+oraclePath+" http://x",
		enc, strings.Replace(enc, "-e ", "-en ", 1), strings.Replace(enc, "-e ", "/enc ", 1), strings.Replace(enc, "-e ", "-EC ", 1),
		"powershell -e not-base64!!", "powershell -enc:not-base64!!",
		"powershell -c Set-Content .infuse/oracle/x y", "pwsh -Com Remove-Item .infuse/oracle/x",
	)
	allowed(t, g, root, "curl http://example.test/x", "curl -o out.bin http://x", "powershell -NoProfile -c Get-Date")
}

func encodePS(s string) string { return b64(utf16LE(s)) }

// The guard's working directory: subshells end, cd - goes back, and -C belongs to the command.
func TestGuardFollowsTheWorkingDirectoryLikeTheShell(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	refused(t, g, root,
		"(cd src); rm "+oraclePath,
		"cd .infuse; cd /; cd -; rm oracle/ready.md",
		"X=1 cd .infuse && rm oracle/ready.md",
	)
	allowed(t, g, root, "(cd .infuse/oracle); rm ready.md")
}

// A refused read and write both leave a "blocked" line in the journal.
func TestRefusalsAreJournaledAsBlocked(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	g := newGuard(root, nil)
	for _, c := range []string{"rm -rf .infuse", "git clean -fdx", "cat .infuse/journal.jsonl", "cat > ../x <<'E'\nb\nE"} {
		before := countBlocked(root)
		if _, _, err := g.GuardCommand(c, root); err == nil {
			t.Fatalf("not refused: %q", c)
		}
		if countBlocked(root) != before+1 {
			t.Errorf("%q left no blocked entry", c)
		}
	}
}

func countBlocked(root string) int {
	n := 0
	for _, e := range journal.ReadJournalSince(root, 0) {
		if e.Kind == "blocked" {
			n++
		}
	}
	return n
}
