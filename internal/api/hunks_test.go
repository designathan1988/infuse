package api

import (
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/workspace"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := workspace.GitRun(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return out
}

// hunkRepo is a repository with three separate edits to one file, none of them staged.
func hunkRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "config", "user.email", "t@example.test")
	gitT(t, dir, "config", "user.name", "T")
	gitT(t, dir, "config", "core.autocrlf", "false")
	var lines []string
	for i := 0; i < 40; i++ {
		lines = append(lines, "line")
	}
	write := func() {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write()
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "base")
	lines[2], lines[20], lines[37] = "first", "second", "third"
	write()
	return dir
}

func read(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "f.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hunkCall(t *testing.T, root, path, id, op string) (int, map[string]any) {
	t.Helper()
	body := map[string]string{"path": path, "id": id, "op": op}
	w := httptest.NewRecorder()
	studioHunk(w, map[string]string{}, root, func(k string) string { return body[k] })
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestSplitDiffSeparatesHeaderAndHunks(t *testing.T) {
	dir := hunkRepo(t)
	head, hunks := splitDiff(gitDiffOf(dir, "f.txt"))
	if !strings.Contains(head, "+++ b/f.txt") || strings.Contains(head, "@@") {
		t.Fatalf("head: %q", head)
	}
	if len(hunks) != 3 {
		t.Fatalf("hunks: %d", len(hunks))
	}
	seen := map[string]bool{}
	for _, h := range hunks {
		if !strings.HasPrefix(h.Header, "@@ ") || seen[h.ID] || h.ID == "" {
			t.Fatalf("hunk %+v", h)
		}
		seen[h.ID] = true
	}
}

func TestRejectRevertsOnlyThatHunk(t *testing.T) {
	dir := hunkRepo(t)
	hunks := hunksOf(gitDiffOf(dir, "f.txt"))
	if code, out := hunkCall(t, dir, "f.txt", hunks[1].ID, "reject"); code != 200 {
		t.Fatalf("reject: %d %v", code, out)
	}
	got := read(t, dir)
	if strings.Contains(got, "second") || !strings.Contains(got, "first") || !strings.Contains(got, "third") {
		t.Fatalf("after reject:\n%s", got)
	}
	if n := len(hunksOf(gitDiffOf(dir, "f.txt"))); n != 2 {
		t.Fatalf("left to review: %d", n)
	}
}

func TestAcceptKeepsTheHunkAndTakesItOutOfReview(t *testing.T) {
	dir := hunkRepo(t)
	hunks := hunksOf(gitDiffOf(dir, "f.txt"))
	if code, out := hunkCall(t, dir, "f.txt", hunks[0].ID, "accept"); code != 200 {
		t.Fatalf("accept: %d %v", code, out)
	}
	if got := read(t, dir); !strings.Contains(got, "first") {
		t.Fatal("accepting changed the file")
	}
	if staged := gitT(t, dir, "diff", "--cached"); !strings.Contains(staged, "+first") || strings.Contains(staged, "+second") {
		t.Fatalf("staged: %s", staged)
	}
	res := gitFileDiff(dir, "f.txt")
	if res["accepted"] != 1 || len(res["hunks"].([]diffHunk)) != 2 {
		t.Fatalf("review: %+v", res)
	}
}

// Every hunk accepted must not read as an untracked file shown whole as added.
func TestFullyAcceptedFileShowsItsDiffNotTheWholeFile(t *testing.T) {
	dir := hunkRepo(t)
	for range 3 {
		h := hunksOf(gitDiffOf(dir, "f.txt"))[0]
		if code, out := hunkCall(t, dir, "f.txt", h.ID, "accept"); code != 200 {
			t.Fatalf("accept: %d %v", code, out)
		}
	}
	res := gitFileDiff(dir, "f.txt")
	diff, _ := res["diff"].(string)
	if res["allAccepted"] != true || strings.Contains(diff, "--- /dev/null") || !strings.Contains(diff, "+second") {
		t.Fatalf("review: %+v", res)
	}
}

func TestStaleOrUnknownHunkIsRefused(t *testing.T) {
	dir := hunkRepo(t)
	hunks := hunksOf(gitDiffOf(dir, "f.txt"))
	before := read(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Replace(before, "second", "edited again", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := hunkCall(t, dir, "f.txt", hunks[1].ID, "reject"); code != http.StatusConflict || out["code"] != "git.hunkStale" {
		t.Fatalf("stale hunk: %d %v", code, out)
	}
	if !strings.Contains(read(t, dir), "edited again") {
		t.Fatal("a stale hunk touched the file")
	}
	if code, _ := hunkCall(t, dir, "f.txt", hunks[0].ID, "bogus"); code != 400 {
		t.Fatalf("unknown op: %d", code)
	}
	if code, _ := hunkCall(t, dir, "", hunks[0].ID, "reject"); code != 400 {
		t.Fatalf("no path: %d", code)
	}
}

func TestPlainFolderRejectsAHunkAndCannotAccept(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\nchanged\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out := hunkCall(t, dir, "f.txt", "x", "accept"); code != 400 || out["code"] != "git.hunkNeedsGit" {
		t.Fatalf("accept in a folder: %d %v", code, out)
	}
	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+changed\n three\n"
	if out, err := applyHunk(dir, "reject", "f.txt", patch); err != nil {
		t.Fatalf("reject: %v %s", err, out)
	}
	if got := read(t, dir); got != "one\ntwo\nthree\n" {
		t.Fatalf("after reject: %q", got)
	}
}

// The local diff is always LF: rejecting must neither fail on a CRLF file nor rewrite its endings.
func TestPlainFolderRejectKeepsCRLF(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("one\r\nchanged\r\nthree\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/f.txt\n+++ b/f.txt\n@@ -1,3 +1,3 @@\n one\n-two\n+changed\n three\n"
	if out, err := applyHunk(dir, "reject", "f.txt", patch); err != nil {
		t.Fatalf("reject: %v %s", err, out)
	}
	if got := read(t, dir); got != "one\r\ntwo\r\nthree\r\n" {
		t.Fatalf("after reject: %q", got)
	}
}

func branchCall(t *testing.T, root, name string) (int, map[string]any) {
	t.Helper()
	w := httptest.NewRecorder()
	studioBranch(w, map[string]string{}, root, func(k string) string {
		if k == "name" {
			return name
		}
		return ""
	})
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestBranchIsCreatedAndWorkComesAlong(t *testing.T) {
	dir := hunkRepo(t)
	if code, out := branchCall(t, dir, "feature/review"); code != 200 || out["branch"] != "feature/review" {
		t.Fatalf("branch: %d %v", code, out)
	}
	if got := strings.TrimSpace(workspace.GitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")); got != "feature/review" {
		t.Fatalf("HEAD on %s", got)
	}
	if !strings.Contains(read(t, dir), "first") {
		t.Fatal("uncommitted work was lost")
	}
	for _, bad := range []string{"", "-x", "--force", "a b", "a..b"} {
		if code, out := branchCall(t, dir, bad); code != 400 || out["code"] != "git.invalidBranch" {
			t.Errorf("branch %q: %d %v", bad, code, out)
		}
	}
	if code, out := branchCall(t, dir, "feature/review"); code != http.StatusConflict {
		t.Errorf("existing branch: %d %v", code, out)
	}
	notRepo := t.TempDir()
	if core.Exists(filepath.Join(notRepo, ".git")) {
		t.Fatal("temp folder is a repository")
	}
	if code, out := branchCall(t, notRepo, "x"); code != 400 || out["code"] != "git.notRepo" {
		t.Errorf("plain folder: %d %v", code, out)
	}
}
