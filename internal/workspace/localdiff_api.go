package workspace

import (
	"fmt"
	"infuse/internal/core"
	"os"
	"path/filepath"
)

// Changes, diff and discard for the file pane.

func LocalChanges(root, work, session, scope string, src SnapshotSource) map[string]any {
	files, _ := localFileChanges(root, work, session, scope, src)
	if files == nil {
		files = []FileChange{}
	}
	add, rem := 0, 0
	for _, f := range files {
		add += f.Added
		rem += f.Removed
	}
	return map[string]any{"isGit": false, "local": true, "scope": core.FirstNonEmpty(scope, "all"), "files": files, "added": add, "removed": rem}
}

// LocalDiff is the unified diff of one file against the scope's base.
func LocalDiff(root, work, session, scope, p string, src SnapshotSource) string {
	_, base := localFileChanges(root, work, session, scope, src)
	b, ok := base[p]
	if !ok {
		return ""
	}
	before, _ := b.text(root)
	now, _ := os.ReadFile(filepath.Join(work, filepath.FromSlash(p)))
	d, _, _ := unifiedDiff(p, core.SplitLines(before), core.SplitLines(string(now)))
	return d
}

// LocalDiscard puts a file back as it was at the start of the scope: rewritten, recreated or removed.
func LocalDiscard(root, work, session, scope, p string, src SnapshotSource) error {
	_, base := localFileChanges(root, work, session, scope, src)
	b, ok := base[p]
	if !ok {
		return fmt.Errorf("file with no recorded change")
	}
	abs := filepath.Join(work, filepath.FromSlash(p))
	if !core.Within(work, abs) {
		return fmt.Errorf("fora do projeto")
	}
	if b.isNew {
		return os.Remove(abs)
	}
	before, ok := b.text(root)
	if !ok {
		return fmt.Errorf("the previous copy no longer exists")
	}
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	return os.WriteFile(abs, []byte(before), 0o644)
}
