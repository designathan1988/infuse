package main

// Review: what changed in the project (per-file line counts), pull requests through the GitHub CLI,
// opening paths in other apps, and user edits from the file pane.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type fileChange struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Added   int    `json:"added"`
	Removed int    `json:"removed"`
}

// Changes lists the working-tree changes against HEAD with line counts (untracked files count as added).
func Changes(dir string) map[string]any {
	if !exists(filepath.Join(dir, ".git")) {
		return map[string]any{"isGit": false, "files": []fileChange{}}
	}
	counts := map[string][2]int{}
	if out, err := gitRun(dir, "diff", "--numstat", "HEAD"); err == nil {
		for _, l := range strings.Split(out, "\n") {
			f := strings.SplitN(l, "\t", 3)
			if len(f) == 3 {
				a, _ := strconv.Atoi(f[0])
				r, _ := strconv.Atoi(f[1])
				counts[f[2]] = [2]int{a, r}
			}
		}
	}
	files := []fileChange{}
	add, rem := 0, 0
	porcelain, _ := gitRun(dir, "status", "--porcelain", "-uall") // not gitOutput: it trims the leading status column
	for _, l := range strings.Split(strings.TrimRight(porcelain, "\n"), "\n") {
		if len(l) < 4 {
			continue
		}
		st := strings.TrimSpace(l[:2])
		p := strings.Trim(strings.TrimSpace(l[3:]), `"`)
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+4:]
		}
		if strings.HasPrefix(p, infuseDirName+"/") {
			continue // the filter's own artifacts are not changes to review
		}
		c := fileChange{Path: p, Status: st}
		if n, ok := counts[p]; ok {
			c.Added, c.Removed = n[0], n[1]
		} else if st == "??" {
			if b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p))); err == nil && len(b) < 4<<20 {
				c.Added = strings.Count(string(b), "\n")
				if len(b) > 0 && b[len(b)-1] != '\n' {
					c.Added++
				}
			}
		}
		add += c.Added
		rem += c.Removed
		files = append(files, c)
	}
	return map[string]any{"isGit": true, "files": files, "added": add, "removed": rem,
		"branch": strings.TrimSpace(gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD"))}
}

func ghAvailable() bool { _, err := exec.LookPath("gh"); return err == nil }

// PRStatus reports the pull request of the current branch (nil when there is none or gh is missing).
func PRStatus(dir string) map[string]any {
	if !ghAvailable() {
		return map[string]any{"gh": false}
	}
	cmd := exec.Command("gh", "pr", "view", "--json", "number,title,url,state,isDraft,reviewDecision,statusCheckRollup,mergeStateStatus")
	noConsoleWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return map[string]any{"gh": true, "pr": nil}
	}
	var pr map[string]any
	_ = jsonUnmarshal(out, &pr)
	return map[string]any{"gh": true, "pr": pr}
}

func ghRun(dir string, args ...string) (string, error) {
	if !ghAvailable() {
		return "", errors.New("GitHub CLI (gh) não encontrado: instale em https://cli.github.com e rode `gh auth login`")
	}
	cmd := exec.Command("gh", args...)
	noConsoleWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// CreatePR pushes the current branch and opens a pull request filled from the commits.
func CreatePR(dir string) (string, error) {
	if out, err := gitRun(dir, "push", "-u", "origin", "HEAD"); err != nil {
		return out, err
	}
	return ghRun(dir, "pr", "create", "--fill")
}

type editorApp struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"-"`
}

// Editors lists the code editors found on this machine, for "Abrir em".
func Editors() []editorApp {
	var out []editorApp
	for _, e := range []struct{ id, name, bin string }{
		{"vscode", "VS Code", "code"}, {"cursor", "Cursor", "cursor"}, {"zed", "Zed", "zed"}, {"windsurf", "Windsurf", "windsurf"},
		{"idea", "IntelliJ IDEA", "idea"}, {"sublime", "Sublime Text", "subl"},
	} {
		names := []string{e.bin}
		if runtime.GOOS == "windows" {
			names = []string{e.bin + ".cmd", e.bin + ".exe", e.bin}
		}
		for _, n := range names {
			if p, err := exec.LookPath(n); err == nil {
				out = append(out, editorApp{e.id, e.name, p})
				break
			}
		}
	}
	if out == nil {
		out = []editorApp{}
	}
	return out
}

func OpenInEditor(id, path string) error {
	for _, e := range Editors() {
		if e.ID == id {
			cmd := exec.Command(e.Path, path)
			noConsoleWindow(cmd)
			return cmd.Start()
		}
	}
	return errors.New("editor não encontrado")
}

// WriteUserFile saves an edit made in the file pane. mtime (ms) guards against overwriting a change
// made on disk meanwhile; force skips that check.
func WriteUserFile(root, rel, content string, mtime int64, force bool) (int64, error) {
	abs := filepath.Join(root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(root, abs); err != nil || strings.HasPrefix(r, "..") {
		return 0, errors.New("fora do projeto")
	}
	if st, err := os.Stat(abs); err == nil && !force && mtime > 0 && st.ModTime().UnixMilli() > mtime {
		return st.ModTime().UnixMilli(), errConflict
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return 0, err
	}
	Journal(root, JournalEntry{Kind: "write", Path: toPosix(rel), Bytes: len(content), Detail: "usuário (editor)"})
	st, _ := os.Stat(abs)
	if st == nil {
		return time.Now().UnixMilli(), nil
	}
	return st.ModTime().UnixMilli(), nil
}

var errConflict = errors.New("o arquivo mudou no disco")

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func jsonMarshalImpl(v any) ([]byte, error) { return json.Marshal(v) }

// FailedCILogs returns the log of the latest failed CI run of the current branch (cut to what matters).
func FailedCILogs(dir string) (string, error) {
	branch := strings.TrimSpace(gitOutput(dir, "branch", "--show-current"))
	if branch == "" {
		return "", errors.New("não há branch atual")
	}
	list, err := ghRun(dir, "run", "list", "--branch", branch, "--limit", "10", "--json", "databaseId,conclusion,name,headSha")
	if err != nil {
		return "", errors.New(strings.TrimSpace(list))
	}
	var runs []struct {
		ID         int64  `json:"databaseId"`
		Conclusion string `json:"conclusion"`
		Name       string `json:"name"`
		HeadSha    string `json:"headSha"`
	}
	_ = json.Unmarshal([]byte(list), &runs)
	for _, r := range runs {
		if r.Conclusion == "failure" || r.Conclusion == "timed_out" || r.Conclusion == "startup_failure" {
			logs, err := ghRun(dir, "run", "view", fmt.Sprint(r.ID), "--log-failed")
			if err != nil && logs == "" {
				return "", err
			}
			if len(logs) > 24000 { // the end of a failed log carries the error
				logs = "[…]\n" + logs[len(logs)-24000:]
			}
			return fmt.Sprintf("Workflow: %s (run %d, commit %.8s)\n\n%s", r.Name, r.ID, r.HeadSha, logs), nil
		}
	}
	return "", errors.New("nenhuma execução de CI com falha neste branch")
}
