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

// precheckShellWrites applies the write policy to a heredoc body before the transform writes it. The
// transform writes on the host, outside the OS sandbox: a target outside the work root is refused, as
// Write refuses it, and never skipped: it would bypass both the sandbox and the turn audit.
func (s *Session) precheckShellWrites(ctx context.Context, intents []shellWriteIntent) Msg {
	for _, in := range intents {
		if !within(realPath(s.workRoot()), realPath(in.Abs)) {
			Journal(s.Root, JournalEntry{Kind: "blocked", Path: toPosix(in.Abs), Detail: "heredoc outside the project"})
			return M("path.outside", "path", toPosix(in.Abs))
		}
		rel := filepath.ToSlash(relTo(s.workRoot(), in.Abs))
		if b := s.banViolationPath(rel); !b.Empty() {
			return b
		}
		before := ""
		if b, err := os.ReadFile(in.Abs); err == nil {
			before = string(b)
		}
		if why := s.judgeShellContent(ctx, rel, before, in.Content); !why.Empty() {
			return why
		}
		ext := strings.ToLower(filepath.Ext(in.Abs))
		if strings.HasSuffix(ext, ".md") {
			continue // prose: the checks below are for code
		}
		if why := s.judgeShellCode(in, rel, ext, before); !why.Empty() {
			return why
		}
	}
	return Msg{}
}

// judgeShellContent is the part of the policy every file passes, prose included.
func (s *Session) judgeShellContent(ctx context.Context, rel, before, after string) Msg {
	if why := s.instructionWriteCheck(rel, before); !why.Empty() {
		return why
	}
	if strings.EqualFold(filepath.Base(rel), "package.json") {
		if refs := newManifestPackages(before, after); len(refs) > 0 {
			if why := verifyPackageRefs(ctx, refs); !why.Empty() {
				return why
			}
		}
	}
	if why := s.largeRemoval(rel, before, after); !why.Empty() {
		return why
	}
	if why := cheatWriteCheck(s.workRoot(), rel, before, after); !why.Empty() {
		if why.ID != "cheat.test_infra" || (before != "" && !s.agentFile(rel)) {
			return why
		}
	}
	if reTestFile.MatchString(rel) && before != "" && !s.agentFile(rel) && testWeakeningEdit(before, after) {
		return M("sentinel.test_edit_blocked", "file", rel)
	}
	if why, changed := baselineChange(rel, before, after); changed {
		return why
	}
	if codeExts[strings.ToLower(filepath.Ext(rel))] {
		if lines := unjustifiedSuppressions(before, after); len(lines) > 0 {
			return M("shortcut.justify", "lines", strings.Join(lines, "\n"))
		}
	}
	return Msg{}
}

// judgeShellCode is the part of the policy for code: plan, contract, docs, conventions, architecture, size.
func (s *Session) judgeShellCode(in shellWriteIntent, rel, ext, before string) Msg {
	if why := s.baseViolation(rel, codeExts[ext], in.Content); !why.Empty() {
		return why
	}
	if codeExts[ext] && !s.planned() {
		return M("gate.shell_write_plan", "files", rel)
	}
	if why := s.contractGate(); codeExts[ext] && !why.Empty() {
		return why
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
	return s.sizeViolation(rel, ext, before, in.Content)
}
