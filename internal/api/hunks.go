package api

// Review of one hunk at a time. The server always recomputes the diff and applies its own copy of the
// chosen hunk, so a client names a change it was shown and never supplies a patch. Accept keeps a hunk
// by staging it (it leaves the unstaged diff); Reject reverts just that hunk in the working tree. A
// plain folder has no index, so there a change is kept by leaving it and only Reject applies.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"infuse/internal/agent"
	"infuse/internal/core"
	"infuse/internal/workspace"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// diffHunk is one @@ section of a file's diff; ID identifies its exact text.
type diffHunk struct {
	ID     string `json:"id"`
	Header string `json:"header"`
	text   string
}

// splitDiff separates a one-file unified diff into its file header and its hunks.
func splitDiff(diff string) (head string, hunks []diffHunk) {
	cur := -1
	for _, l := range strings.SplitAfter(diff, "\n") {
		switch {
		case strings.HasPrefix(l, "@@ "):
			hunks = append(hunks, diffHunk{Header: strings.TrimRight(l, "\r\n"), text: l})
			cur = len(hunks) - 1
		case cur >= 0:
			hunks[cur].text += l
		default:
			head += l
		}
	}
	for i := range hunks {
		sum := sha256.Sum256([]byte(hunks[i].text))
		hunks[i].ID = hex.EncodeToString(sum[:8])
	}
	return head, hunks
}

func hunksOf(diff string) []diffHunk {
	_, hunks := splitDiff(diff)
	if hunks == nil {
		return []diffHunk{}
	}
	return hunks
}

// gitDiffOf is `git diff` of one path, independent of the user's diff configuration: the output is
// fed back to `git apply`, so prefixes must be the standard ones and warnings must stay out of it.
func gitDiffOf(root, path string, extra ...string) string {
	args := []string{"-c", "core.safecrlf=false", "-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false",
		"--literal-pathspecs", "diff", "--no-ext-diff", "--no-color"}
	args = append(append(args, extra...), "--", path)
	out, _ := workspace.GitRun(root, args...)
	return out
}

// reviewDiff is the diff the review pane shows for one file: against the index in a repository,
// against the kept copy in a plain folder.
func reviewDiff(q map[string]string, root, path string) string {
	if core.Exists(filepath.Join(root, ".git")) {
		return gitDiffOf(root, path)
	}
	return workspace.LocalDiff(core.FirstNonEmpty(q["project"], root), root, q["session"], q["scope"], path, snapshots{})
}

// gitFileDiff is what is left to review in a file, and how much of it was already accepted.
func gitFileDiff(root, path string) map[string]any {
	out := gitDiffOf(root, path)
	staged := gitDiffOf(root, path, "--cached")
	accepted := len(hunksOf(staged))
	if strings.TrimSpace(out) != "" {
		return map[string]any{"path": path, "diff": out, "hunks": hunksOf(out), "accepted": accepted}
	}
	if strings.TrimSpace(staged) != "" { // every hunk accepted: shown for reference, nothing left to decide
		return map[string]any{"path": path, "diff": staged, "hunks": []diffHunk{}, "accepted": accepted, "allAccepted": true}
	}
	// untracked: show whole file as added
	if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err == nil {
		lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
		for i := range lines {
			lines[i] = "+" + lines[i]
		}
		out = "--- /dev/null\n+++ b/" + path + "\n" + strings.Join(lines, "\n")
	}
	return map[string]any{"path": path, "diff": out, "hunks": []diffHunk{}, "accepted": 0}
}

// hunkApply is what each decision runs.
var hunkApply = map[string][]string{
	"accept": {"apply", "--cached", "--whitespace=nowarn"},
	"reject": {"apply", "--reverse", "--whitespace=nowarn"},
}

// eolConfig keeps a plain folder's line endings: outside a repository git has only the user's global
// autocrlf to go by, which would rewrite every line of an LF file, and the local diff is always LF.
func eolConfig(root, path string) []string {
	if core.Exists(filepath.Join(root, ".git")) {
		return nil
	}
	if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))); err == nil && bytes.Contains(b, []byte{'\r', '\n'}) {
		return []string{"-c", "core.autocrlf=true", "-c", "core.safecrlf=false"}
	}
	return []string{"-c", "core.autocrlf=false"}
}

// applyHunk writes the patch to a file outside the project and hands it to git.
func applyHunk(root, op, path, patch string) (string, error) {
	f, err := os.CreateTemp("", "infuse-hunk-*.patch")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	_, err = f.WriteString(patch)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return "", err
	}
	return workspace.GitRun(root, append(append(eolConfig(root, path), hunkApply[op]...), f.Name())...)
}

func findHunk(diff, id string) (string, error) {
	head, hunks := splitDiff(diff)
	if strings.Contains(diff, "\ndiff --git ") { // diff lines are prefixed, so this is a second file
		return "", errors.New("the path is not a single file")
	}
	for _, h := range hunks {
		if h.ID == id {
			return head + h.text, nil
		}
	}
	return "", errHunkStale
}

var errHunkStale = errors.New("the file changed; review the diff again")

// studioHunk is POST /api/git/hunk: {path, id, op: accept|reject}.
func studioHunk(w http.ResponseWriter, q map[string]string, root string, str func(string) string) {
	op, path := str("op"), str("path")
	if hunkApply[op] == nil || path == "" {
		sendJSON(w, 400, map[string]any{"error": "unknown hunk action", "code": "git.hunkInvalid"})
		return
	}
	if op == "accept" && !core.Exists(filepath.Join(root, ".git")) {
		sendJSON(w, 400, map[string]any{"error": "accepting a hunk needs a git repository", "code": "git.hunkNeedsGit"})
		return
	}
	patch, err := findHunk(reviewDiff(q, root, path), str("id"))
	if err == errHunkStale {
		sendJSON(w, 409, map[string]any{"error": err.Error(), "code": "git.hunkStale"})
		return
	}
	if err != nil {
		sendJSON(w, 400, map[string]any{"error": err.Error(), "code": "git.hunkInvalid"})
		return
	}
	out, err := applyHunk(root, op, path, patch)
	if err != nil {
		sendJSON(w, 409, map[string]any{"error": strings.TrimSpace(out), "code": "git.hunkFailed", "args": map[string]any{"error": strings.TrimSpace(out)}})
		return
	}
	sendJSON(w, 200, map[string]any{"ok": true})
}

// studioBranch is POST /api/git/branch: a new branch from HEAD; uncommitted work comes along.
func studioBranch(w http.ResponseWriter, q map[string]string, root string, str func(string) string) {
	name := strings.TrimSpace(str("name"))
	if !core.Exists(filepath.Join(root, ".git")) {
		sendJSON(w, 400, map[string]any{"error": "the folder is not a git repository", "code": "git.notRepo"})
		return
	}
	if _, err := workspace.GitRun(root, "check-ref-format", "--branch", name); err != nil || name == "" {
		sendJSON(w, 400, map[string]any{"error": "invalid branch name", "code": "git.invalidBranch"})
		return
	}
	if out, err := workspace.GitRun(root, "switch", "-c", name); err != nil {
		sendJSON(w, 409, map[string]any{"error": strings.TrimSpace(out), "code": "git.branchFailed", "args": map[string]any{"error": strings.TrimSpace(out)}})
		return
	}
	if id := q["session"]; id != "" { // the session's rail names the branch it works on
		if s, err := agent.LoadSession(core.FirstNonEmpty(q["project"], root), id); err == nil {
			s.Mu.Lock()
			s.Branch = name
			s.Mu.Unlock()
			s.Save()
			s.Emit("session", s.Snapshot())
		}
	}
	sendJSON(w, 200, map[string]any{"ok": true, "branch": name})
}
