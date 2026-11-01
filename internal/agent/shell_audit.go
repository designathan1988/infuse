package agent

// Shell-write audit: every file the tree gained or lost during a turn is judged by the same
// write-time policy, whoever wrote it. Write and Edit pass through the gates as they run; a shell
// command, an MCP tool, a subagent, a code generator or another process changes the files directly,
// so the turn cannot end until those changes are checked too.

import (
	"os"
	"path/filepath"
	"strings"

	"infuse/internal/workspace"
)

// ungatedChanges lists what changed in the work tree since the turn started, minus the paths the
// write gates judged. A checkpoint alone is not a gate: a heredoc written by the filter records one
// before the command runs, and it must still be judged here.
func (s *Session) ungatedChanges() []workspace.TreeChange {
	s.Mu.Lock()
	gated := make(map[string]bool, len(s.gate.gatedPaths))
	for p := range s.gate.gatedPaths {
		gated[p] = true
	}
	s.Mu.Unlock()
	all := workspace.TurnChanges(s.Root, s.workRoot(), s.ID, s.Turn)
	out := all[:0]
	for _, ch := range all {
		if !gated[ch.Path] && !strings.HasPrefix(ch.Path, ".infuse/") {
			out = append(out, ch)
		}
	}
	return out
}

// auditShellWrites returns what the agent must fix before the turn may end (empty when nothing).
func (s *Session) auditShellWrites() Msg {
	if p := workspace.TurnProtectedChanges(s.Root, s.ID, s.Turn); len(p) > 0 {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "protected file changed outside the executor: " + strings.Join(firstN(p, 6), ", ")})
		return M("oracle.protected_write", "files", strings.Join(firstN(p, 6), ", "))
	}
	changes := s.ungatedChanges()
	if len(changes) == 0 {
		return Msg{}
	}
	source := false
	for _, ch := range changes {
		if codeExts[strings.ToLower(filepath.Ext(ch.Path))] {
			source = true
		}
	}
	if source && !s.planned() {
		return M("gate.shell_write_plan", "files", strings.Join(changePaths(changes), ", "))
	}
	var violations []string
	for _, ch := range changes {
		if m := s.shellChangeViolation(ch); !m.Empty() {
			violations = append(violations, ch.Path+": "+m.Render(s.uiLang()))
			Journal(s.Root, JournalEntry{Kind: "gate", Path: ch.Path, Detail: "shell write refused: " + m.Render("en")})
		}
	}
	if len(violations) == 0 {
		return Msg{}
	}
	return M("gate.shell_write", "items", strings.Join(violations, "\n"))
}

// shellChangeViolation runs the write-time policy on one change made outside the write tools.
func (s *Session) shellChangeViolation(ch workspace.TreeChange) Msg {
	rel := filepath.ToSlash(ch.Path)
	if b := s.banViolationPath(rel); !b.Empty() { // the user's prohibition binds every writer
		return b
	}
	before := ""
	if !ch.New {
		if b, ok := workspace.ReadBlob(s.Root, ch.Before); ok {
			before = b
		}
	}
	if ch.Deleted { // a deletion is measured against what it removed
		if why := s.largeRemoval(rel, before, ""); !why.Empty() {
			return why
		}
		return Msg{}
	}
	abs := filepath.Join(s.workRoot(), filepath.FromSlash(rel))
	after, err := os.ReadFile(abs)
	if err != nil {
		return Msg{}
	}
	if why := s.largeRemoval(rel, before, string(after)); !why.Empty() {
		return why
	}
	if _, changed := baselineChange(rel, before, string(after)); changed {
		return M("work.baseline_shell", "file", rel, "kind", baselineKind(rel))
	}
	if why := cheatWriteCheck(s.workRoot(), rel, before, string(after)); !why.Empty() {
		return why
	}
	if lines := unjustifiedSuppressions(before, string(after)); len(lines) > 0 {
		return M("shortcut.justify", "lines", strings.Join(lines, "\n"))
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if strings.HasSuffix(ext, ".md") {
		return Msg{} // prose: a baseline and a marker are judged above; layout and size are not
	}
	if why := s.baseViolation(rel, codeExts[ext]); !why.Empty() {
		return why
	}
	if s.settings.EnforceConventions == nil || *s.settings.EnforceConventions {
		if why := conventionViolation(s.Root, abs, before, string(after)); !why.Empty() {
			return M("protocol.convention", "why", why)
		}
	}
	if s.settings.EnforceArchitecture == nil || *s.settings.EnforceArchitecture {
		if why := archCheck(s.Root, s.workRoot(), rel, before, string(after)); !why.Empty() {
			return M("protocol.architecture", "why", why)
		}
	}
	return s.sizeViolation(rel, ext, before, string(after))
}

func (s *Session) planned() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.gate.planned
}

func changePaths(changes []workspace.TreeChange) []string {
	out := make([]string, 0, len(changes))
	for _, ch := range changes {
		out = append(out, ch.Path)
	}
	if len(out) > 12 {
		return append(out[:12], "…")
	}
	return out
}
