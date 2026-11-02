// Package workspace groups the repository-facing operations: git helpers, worktrees, local changes and
// snapshots, editors and dev servers.
package workspace

import "infuse/internal/platform/gitx"

// GitRun runs git in root and returns its combined output. See gitx: every invocation neutralizes the
// programs a repository's configuration can name.
func GitRun(root string, args ...string) (string, error) { return gitx.Run(root, args...) }

// GitOutput runs git in root and returns its trimmed stdout ("" on failure).
func GitOutput(root string, args ...string) string { return gitx.Output(root, args...) }
