package agent

// Shell writes whose content is known before the command runs (heredocs) are judged by the same
// write-time policy as Write and Edit, before anything touches disk. Content produced only at run
// time is judged at the end of the turn by shell_audit.go.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type shellWriteIntent struct {
	Abs     string
	Content string
}

// precheckShellWrites applies the write policy to a heredoc body before the transform writes it.
func (s *Session) precheckShellWrites(ctx context.Context, intents []shellWriteIntent) Msg {
	for _, in := range intents {
		rel := filepath.ToSlash(relTo(s.workRoot(), in.Abs))
		if rel == "" || strings.HasPrefix(rel, "..") {
			// The filter writes a heredoc in-process, outside the shell sandbox: an out-of-root
			// target would bypass both the sandbox and the turn audit.
			return M("gate.heredoc_outside", "file", in.Abs)
		}
		if b := s.banViolationPath(rel); !b.Empty() {
			return b
		}
		before := ""
		if b, err := os.ReadFile(in.Abs); err == nil {
			before = string(b)
		}
		if why := s.largeRemoval(rel, before, in.Content); !why.Empty() {
			return why
		}
		if why := cheatWriteCheck(s.workRoot(), rel, before, in.Content); !why.Empty() {
			if why.ID != "cheat.test_infra" || (before != "" && !s.agentFile(rel)) {
				return why
			}
		}
		if reTestFile.MatchString(rel) && before != "" && !s.agentFile(rel) && testWeakeningEdit(before, in.Content) {
			return M("sentinel.test_edit_blocked", "file", rel)
		}
		if why, changed := baselineChange(rel, before, in.Content); changed {
			return why
		}
		if lines := unjustifiedSuppressions(before, in.Content); len(lines) > 0 {
			return M("shortcut.justify", "lines", strings.Join(lines, "\n"))
		}
		ext := strings.ToLower(filepath.Ext(in.Abs))
		if strings.HasSuffix(ext, ".md") {
			continue // prose: the checks below are for code
		}
		if why := s.baseViolation(rel, codeExts[ext]); !why.Empty() {
			return why
		}
		if codeExts[ext] && !s.planned() {
			return M("gate.shell_write_plan", "files", rel)
		}
		if m := s.docsBeforeUse(in.Abs, before, in.Content); !m.Empty() {
			return m
		}
		if s.settings.EnforceConventions == nil || *s.settings.EnforceConventions {
			if why := conventionViolation(s.Root, in.Abs, before, in.Content); !why.Empty() {
				return M("protocol.convention", "why", why)
			}
		}
		if s.settings.EnforceArchitecture == nil || *s.settings.EnforceArchitecture {
			if why := archCheck(s.Root, s.workRoot(), rel, before, in.Content); !why.Empty() {
				return M("protocol.architecture", "why", why)
			}
		}
		if why := s.sizeViolation(rel, ext, before, in.Content); !why.Empty() {
			return why
		}
	}
	return Msg{}
}
