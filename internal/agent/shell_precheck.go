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
		if why := s.heredocShape(rel, in.Abs, before, in.Content); !why.Empty() {
			return why
		}
	}
	return Msg{}
}

func (s *Session) heredocShape(rel, abs, before, after string) Msg {
	if why := s.instructionWriteCheck(rel, before); !why.Empty() {
		return why
	}
	if strings.EqualFold(filepath.Base(rel), "package.json") {
		if refs := newManifestPackages(before, after); len(refs) > 0 {
			if why := verifyPackageRefs(context.Background(), refs); !why.Empty() {
				return why
			}
		}
	}
	ext := strings.ToLower(filepath.Ext(abs))
	if codeExts[ext] {
		if lines := unjustifiedSuppressions(before, after); len(lines) > 0 {
			return M("shortcut.justify", "lines", strings.Join(lines, "\n"))
		}
	}
	if strings.HasSuffix(ext, ".md") {
		return Msg{} // prose: the checks below are for code
	}
	if why := s.baseViolation(rel, codeExts[ext]); !why.Empty() {
		return why
	}
	if codeExts[ext] && !s.planned() {
		return M("gate.shell_write_plan", "files", rel)
	}
	if m := s.docsBeforeUse(abs, before, after); !m.Empty() {
		return m
	}
	if s.settings.EnforceConventions == nil || *s.settings.EnforceConventions {
		if why := conventionViolation(s.Root, abs, before, after); !why.Empty() {
			return M("protocol.convention", "why", why)
		}
	}
	if s.settings.EnforceArchitecture == nil || *s.settings.EnforceArchitecture {
		if why := archCheck(s.Root, s.workRoot(), rel, before, after); !why.Empty() {
			return M("protocol.architecture", "why", why)
		}
	}
	return s.sizeViolation(rel, ext, before, after)
}
