package oracle

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"infuse/internal/core"
	"infuse/internal/platform/process"
	"infuse/internal/sandbox"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, root string) Verdict {
	t.Helper()
	v, err := RunOracle(context.Background(), root, root, "")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// specJSON builds criteria from one command check.
func commandSpec(script string) string {
	b, _ := json.Marshal(map[string]any{"checks": []any{map[string]any{"id": "c", "kind": "command", "run": script}}})
	return string(b)
}

func requireContainment(t *testing.T) {
	t.Helper()
	if !sandbox.Available() || core.FindRealBash() == "" {
		t.Skip("this platform has no OS containment or no bash")
	}
}

// A malicious test runs with the snapshot as its only writable tree: the project's criteria, the
// user's store and the user's settings are out of its reach, and its environment names no infuse.
func TestMaliciousCheckCannotReachTheUserStore(t *testing.T) {
	testHome(t)
	requireContainment(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	home := core.InfuseHome()
	targets := []string{SpecFile(root), filepath.Join(home, "settings.json"), filepath.Join(home, "oracle", "forged.json")}
	script := "test -f a.txt"
	for _, p := range targets {
		script += " && ! (echo forged >> '" + filepath.ToSlash(p) + "') 2>/dev/null"
	}
	writeSpec(t, root, commandSpec(script))
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("a contained check wrote where it must not: %+v", v)
	}
	for _, p := range targets[1:] {
		if core.Exists(p) {
			t.Fatalf("%s was created by a check", p)
		}
	}
}

// INFUSE_ORACLE was a switch a test could read to behave differently under the executor.
func TestChecksCannotDetectTheExecutorFromTheEnvironment(t *testing.T) {
	testHome(t)
	requireContainment(t)
	root := t.TempDir()
	writeSpec(t, root, commandSpec(`if env | grep -i '^INFUSE'; then exit 1; fi; pwd | grep -vi 'oracle'`))
	if v := run(t, root); v.Verdict != "aprovado" {
		log, _ := os.ReadFile(filepath.Join(EvidenceDir(root), v.Run, "c.log"))
		t.Fatalf("the executor is detectable: %s", log)
	}
}

// Adding tests/ to .gitignore, or dropping it from the index, must not remove it from the snapshot.
func TestSnapshotKeepsFilesGitWouldDrop(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	mustWrite(t, filepath.Join(root, "tests", "a.test"), "kept\n")
	mustWrite(t, filepath.Join(root, "dist", "tracked.js"), "kept\n")
	if out, err := runGit(root, "add", "-A", "-f"); err != nil {
		t.Fatal(out)
	}
	if out, err := runGit(root, "commit", "-qm", "tests"); err != nil {
		t.Fatal(out)
	}
	mustWrite(t, filepath.Join(root, ".gitignore"), "tests/\n.env\n")
	mustWrite(t, filepath.Join(root, ".env"), "SECRET=1\n")
	if out, err := runGit(root, "rm", "-r", "-q", "--cached", "tests"); err != nil {
		t.Fatal(out)
	}
	writeSpec(t, root, `{"checks":[
		{"id":"tests","kind":"file","path":"tests/a.test","expect":{"contains":"kept"}},
		{"id":"build","kind":"file","path":"dist/tracked.js"},
		{"id":"env","kind":"file","path":".env","expect":{"exists":false}}
	]}`)
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("the snapshot lost files: %+v", v.Checks)
	}
}

// Taking a snapshot goes through no checkout: a planted post-checkout hook has nothing to fire on.
func TestSnapshotFiresNoHook(t *testing.T) {
	testHome(t)
	root := gitRepo(t)
	marker := filepath.Join(t.TempDir(), "hook-ran")
	hook := "#!/bin/sh\necho ran > '" + filepath.ToSlash(marker) + "'\n"
	for _, name := range []string{"post-checkout", "pre-commit", "post-commit"} {
		mustWrite(t, filepath.Join(root, ".git", "hooks", name), hook)
	}
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("%+v", v)
	}
	if core.Exists(marker) {
		t.Fatal("a hook planted in the project ran on the host")
	}
}

// A link pointing out of the project is not followed: the copy must not hand a check the user's files.
func TestLinkOutsideTheProjectIsNotCopied(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.txt")
	mustWrite(t, outside, "secret\n")
	if err := os.Symlink(outside, filepath.Join(root, "leak.txt")); err != nil {
		t.Skipf("no symlink privilege: %v", err)
	}
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	writeSpec(t, root, `{"checks":[{"id":"gone","kind":"file","path":"leak.txt","expect":{"exists":false}}]}`)
	if v := run(t, root); v.Verdict != "aprovado" || len(v.Warnings) == 0 {
		t.Fatalf("%+v", v)
	}
}

// A file the executor cannot read stops the run: leaving it out is how a test disappears.
func TestSnapshotFailsClosedOnAnUnreadableFile(t *testing.T) {
	c := newCopier(t.TempDir(), t.TempDir())
	c.files = []string{"missing.txt"}
	if _, err := c.copyFiles(); err == nil {
		t.Fatal("a file that cannot be read must stop the snapshot")
	}
}

// Removing the file is the cheapest way to disarm the oracle.
func TestDeletedCriteriaAreRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	if err := os.Remove(SpecFile(root)); err != nil {
		t.Fatal(err)
	}
	if v := run(t, root); v.Verdict != "erro" {
		t.Fatalf("deleted criteria must not read as no criteria: %+v", v)
	}
}

// Only saving creates an anchor: a file with no anchor was not written by the user's editor.
func TestCriteriaWithoutAnchorAreRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	mustWrite(t, SpecFile(root), `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	if v := run(t, root); v.Verdict != "erro" || len(v.Checks) != 0 {
		t.Fatalf("unanchored criteria ran: %+v", v)
	}
	if core.Exists(anchorFile(root)) {
		t.Fatal("the run created the anchor it should have required")
	}
}

// The user clearing the criteria is not a removal: the anchor says so.
func TestCriteriaSavedEmptyAreNoCriteria(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	writeSpec(t, root, `{"checks":[]}`)
	if v := run(t, root); v.Verdict != "sem-criterios" {
		t.Fatalf("%+v", v)
	}
	_ = os.Remove(SpecFile(root))
	if v := run(t, root); v.Verdict != "sem-criterios" {
		t.Fatalf("%+v", v)
	}
}

func copyRun(t *testing.T, from, to string) {
	t.Helper()
	entries, err := os.ReadDir(from)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if b, err := os.ReadFile(filepath.Join(from, e.Name())); err == nil {
			mustWrite(t, filepath.Join(to, e.Name()), string(b))
		}
	}
}

// An old approved run copied under a newer name keeps a valid signature and must still not verify.
func TestCopiedVerdictDoesNotVerify(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	v := run(t, root)
	if list := ListVerdicts(root, 1); !list[0].SpecVerified {
		t.Fatalf("the genuine run must verify: %+v", list[0])
	}
	copyRun(t, filepath.Join(EvidenceDir(root), v.Run), filepath.Join(EvidenceDir(root), "99999999-999999.999"))
	if list := ListVerdicts(root, 1); list[0].SpecVerified {
		t.Fatal("a copied run verified as the newest")
	}
}

// A verdict moved to another project names the project it came from.
func TestVerdictOfAnotherProjectDoesNotVerify(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	v := run(t, root)
	other := t.TempDir()
	copyRun(t, filepath.Join(EvidenceDir(root), v.Run), filepath.Join(EvidenceDir(other), v.Run))
	if list := ListVerdicts(other, 1); len(list) != 1 || list[0].SpecVerified {
		t.Fatalf("%+v", list)
	}
}

// The signature covers the fingerprint, and the fingerprint covers the manifest beside the verdict.
func TestEditedManifestDoesNotVerify(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.txt"), "x\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"a.txt"}]}`)
	v := run(t, root)
	if v.Fingerprint == "" {
		t.Fatal("the verdict has no fingerprint")
	}
	mustWrite(t, filepath.Join(EvidenceDir(root), v.Run, "snapshot.txt"), "edited\n")
	if list := ListVerdicts(root, 1); list[0].SpecVerified {
		t.Fatal("an edited manifest verified")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// localhost can resolve to either stack: a listener only on ::1 is still an impostor.
func TestPortFreeSeesTheIPv6Loopback(t *testing.T) {
	ln, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skip("no IPv6 loopback")
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if portFree("http://localhost:"+strconv.Itoa(port)+"/") == nil {
		t.Fatal("a listener on ::1 went unnoticed")
	}
}

// Every address the checks read is verified, not only the first.
func TestEveryCheckedPortIsVerifiedFree(t *testing.T) {
	testHome(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	root := t.TempDir()
	spec := `{"server":"sleep 30","checks":[
		{"id":"one","kind":"http","url":"http://127.0.0.1:` + strconv.Itoa(freePort(t)) + `/"},
		{"id":"two","kind":"http","url":"http://127.0.0.1:` + strconv.Itoa(busy) + `/"}]}`
	writeSpec(t, root, spec)
	if v := run(t, root); v.Verdict != "erro" || !strings.Contains(v.Summary, strconv.Itoa(busy)) {
		t.Fatalf("%+v", v)
	}
}

// A listener outside the started tree is refused even when it answers: this is the impostor that
// arrived after the port was seen free.
func TestListenerOutsideTheStartedTreeIsRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cmd := helperSleep(t)
	url := "http://127.0.0.1:" + strconv.Itoa(ln.Addr().(*net.TCPAddr).Port) + "/"
	if err := verifyListeners([]string{url}, cmd); err == nil {
		t.Fatal("a listener of another process was accepted as the server")
	}
}

func helperSleep(t *testing.T) *exec.Cmd {
	t.Helper()
	bash := core.FindRealBash()
	if bash == "" {
		t.Skip("no bash")
	}
	cmd := exec.Command(bash, "-c", "sleep 30")
	process.ProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	process.AttachJob(cmd)
	t.Cleanup(func() { process.KillTree(cmd); _ = cmd.Wait() })
	return cmd
}
