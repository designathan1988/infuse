package main

// Session worktrees: a session can run in its own git worktree (a separate checkout on its own branch)
// so parallel sessions never touch each other's files. Worktrees live under the project, in
// settings.WorktreeDir (default .infuse/worktrees), and that folder is kept out of git status.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var reBranchUnsafe = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// CreateWorktree makes a new worktree for a session and returns its path and branch.
func CreateWorktree(root, name string) (string, string, error) {
	if !exists(filepath.Join(root, ".git")) {
		return "", "", errors.New("a pasta não é um repositório git; worktree exige git")
	}
	if strings.TrimSpace(gitOutput(root, "rev-parse", "--verify", "HEAD")) == "" {
		return "", "", errors.New("o repositório ainda não tem nenhum commit")
	}
	s := LoadSettings()
	slug := strings.Trim(reBranchUnsafe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		slug = "sessao"
	}
	if len(slug) > 40 {
		slug = slug[:40]
	}
	slug = fmt.Sprintf("%s-%s", slug, time.Now().Format("0102-150405"))
	dir := filepath.Join(root, filepath.FromSlash(s.WorktreeDir), slug)
	branch := s.BranchPrefix + slug
	excludeFromGit(root, s.WorktreeDir)
	if out, err := gitRun(root, "worktree", "add", "-b", branch, dir, "HEAD"); err != nil {
		return "", "", fmt.Errorf("git worktree add: %s", strings.TrimSpace(out))
	}
	return dir, branch, nil
}

// RemoveWorktree deletes a session's worktree (its branch stays, with whatever was committed).
func RemoveWorktree(root, dir string, force bool) error {
	args := []string{"worktree", "remove", dir}
	if force {
		args = []string{"worktree", "remove", "--force", dir}
	}
	if out, err := gitRun(root, args...); err != nil {
		return fmt.Errorf("%s", strings.TrimSpace(out))
	}
	return nil
}

// excludeFromGit adds the worktree folder to .git/info/exclude so it never shows as a change.
func excludeFromGit(root, rel string) {
	file := filepath.Join(root, ".git", "info", "exclude")
	line := "/" + strings.Trim(filepath.ToSlash(rel), "/") + "/"
	b, _ := os.ReadFile(file)
	if strings.Contains(string(b), line) {
		return
	}
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(b) > 0 && b[len(b)-1] != '\n' {
		_, _ = f.WriteString("\n")
	}
	_, _ = f.WriteString(line + "\n")
}
