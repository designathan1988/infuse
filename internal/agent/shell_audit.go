package agent

// Shell-write audit: every file the tree gained or lost during a turn is judged by the same
// write-time policy, whoever wrote it. Write and Edit pass through the gates as they run; a shell
// command, an MCP tool, a subagent, a code generator or another process changes the files directly,
// so the turn cannot end until those changes are checked too.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	gated := make(map[string]string, len(s.gate.gatedPaths))
	for p, d := range s.gate.gatedPaths {
		gated[p] = d
	}
	s.Mu.Unlock()
	all := workspace.TurnChanges(s.Root, s.workRoot(), s.ID, s.Turn)
	out := all[:0]
	for _, ch := range all {
		// infuse writes its own records; a deletion of one, however, must reach the protected-record
		// check below even though the file no longer exists.
		if !ch.Deleted && gated[ch.Path] == fileDigest(filepath.Join(s.workRoot(), filepath.FromSlash(ch.Path))) {
			continue // the exact content a write gate approved; a later rewrite does not match
		}
		if strings.HasPrefix(ch.Path, ".infuse/") && !ch.Deleted {
			continue
		}
		out = append(out, ch)
	}
	return out
}

// markGated records the digest a successful write tool left at rel. The shell audit may skip that
// exact content; the same path written again by anything else stays under audit.
func (s *Session) markGated(rel string) {
	sum := fileDigest(filepath.Join(s.workRoot(), filepath.FromSlash(rel)))
	if sum == "" {
		return
	}
	s.Mu.Lock()
	if s.gate.gatedPaths == nil {
		s.gate.gatedPaths = map[string]string{}
	}
	s.gate.gatedPaths[rel] = sum
	s.Mu.Unlock()
}

func fileDigest(abs string) string {
	b, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// auditShellWrites returns what the agent must fix before the turn may end (empty when nothing).
func (s *Session) auditShellWrites() Msg {
	if p := workspace.TurnProtectedChanges(s.Root, s.ID, s.Turn); len(p) > 0 {
		workspace.RestoreProtected(s.Root, s.ID, s.Turn)
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "protected file changed outside the executor (restored): " + strings.Join(firstN(p, 6), ", ")})
		return M("oracle.protected_write", "files", strings.Join(firstN(p, 6), ", "))
	}
	changes := s.ungatedChanges()
	if len(changes) == 0 {
		return Msg{}
	}
	// A change made outside the write tools counts as a change for declared verification too, so a
	// command that ran before the shell edit cannot satisfy the project's after-change check.
	s.Mu.Lock()
	if !s.gate.shellCounted {
		s.gate.writes++
		s.gate.shellCounted = true
	}
	s.Mu.Unlock()
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

// restoreShellChange puts back the user's test or test infrastructure after a shell write tried to
// alter it. The turn is still refused; the restored file is what the next turn sees.
func (s *Session) restoreShellChange(abs, before string) {
	if before == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(abs, []byte(before), 0o644)
	Journal(s.Root, JournalEntry{Kind: "gate", Path: relTo(s.workRoot(), abs), Detail: "restored after a shell change to a user test or test infrastructure"})
}

// protectedRecord recognizes the control records whose deletion must be visible even when the file no
// longer exists for the snapshot comparison to list it.
func protectedRecord(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch {
	case rel == ".infuse", strings.HasPrefix(rel, ".infuse/"):
		return true
	case rel == ".git", strings.HasPrefix(rel, ".git/"):
		return true
	case rel == ".mcp.json", rel == ".claude/settings.json":
		return true
	}
	return false
}

// shellTestGuard applies the user-test rule to a shell change and restores a weakened test.
func (s *Session) shellTestGuard(rel, abs, before, after string, newFile, deleted bool) Msg {
	if newFile && (reTestFile.MatchString(rel) || reTestInfra.MatchString(rel)) {
		s.markAgentFile(rel) // the agent created this test or its infrastructure through the shell
		return Msg{}
	}
	if !reTestFile.MatchString(rel) || s.agentFile(rel) || before == "" {
		return Msg{}
	}
	if deleted || testWeakeningEdit(before, after) {
		s.restoreShellChange(abs, before)
		return M("sentinel.test_edit_blocked", "file", rel)
	}
	return Msg{}
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
	abs := filepath.Join(s.workRoot(), filepath.FromSlash(rel))
	if why := s.shellRecordViolation(rel, abs, before, ch.New); !why.Empty() {
		return why
	}
	if ch.Deleted {
		return s.shellDeletionViolation(rel, abs, before)
	}
	after, err := os.ReadFile(abs)
	if err != nil {
		return Msg{}
	}
	afterText := string(after)
	if strings.EqualFold(filepath.Base(rel), "package.json") {
		if refs := newManifestPackages(before, afterText); len(refs) > 0 {
			if why := verifyPackageRefs(context.Background(), refs); !why.Empty() {
				return why
			}
		}
	}
	if m := s.shellTestGuard(rel, abs, before, afterText, ch.New, false); !m.Empty() {
		return m
	}
	if why := s.largeRemoval(rel, before, afterText); !why.Empty() {
		return why
	}
	if _, changed := baselineChange(rel, before, afterText); changed {
		return M("work.baseline_shell", "file", rel, "kind", baselineKind(rel))
	}
	if why := cheatWriteCheck(s.workRoot(), rel, before, afterText); !why.Empty() {
		if why.ID == "cheat.test_infra" && !s.agentFile(rel) && before != "" {
			s.restoreShellChange(abs, before)
		}
		return why
	}
	why := s.shellStyleViolation(rel, abs, before, after)
	if why.Empty() {
		s.noteUIChange(rel) // a screen edited through the shell is still a changed screen
	}
	return why
}

// shellRecordViolation protects the user's control records from every writer: a rewrite is removed or
// restored before the refusal reaches the agent.
func (s *Session) shellRecordViolation(rel, abs, before string, isNew bool) Msg {
	why := Msg{}
	if protectedRecord(rel) { // .mcp.json, .claude/settings.json: a modification is as serious as a deletion
		why = M("oracle.protected_write", "files", rel)
	} else if m := s.instructionWriteCheck(rel, before); !m.Empty() {
		why = m
	}
	if why.Empty() {
		return why
	}
	if isNew {
		_ = os.Remove(abs)
	} else {
		s.restoreShellChange(abs, before)
	}
	return why
}

// shellDeletionViolation judges a file the shell removed.
func (s *Session) shellDeletionViolation(rel, abs, before string) Msg {
	if kind := baselineKind(rel); kind != "" {
		s.restoreShellChange(abs, before)
		return M("work.baseline", "file", rel, "kind", kind)
	}
	if m := s.shellTestGuard(rel, abs, before, "", false, true); !m.Empty() {
		return m
	}
	return s.largeRemoval(rel, before, "")
}

// shellStyleViolation judges the shape of a shell-written file: suppressions, base placement,
// conventions, architecture and size.
func (s *Session) shellStyleViolation(rel, abs, before string, after []byte) Msg {
	text := string(after)
	ext := strings.ToLower(filepath.Ext(abs))
	if codeExts[ext] {
		if lines := unjustifiedSuppressions(before, text); len(lines) > 0 {
			return M("shortcut.justify", "lines", strings.Join(lines, "\n"))
		}
	}
	if strings.HasSuffix(ext, ".md") {
		return Msg{} // prose: a baseline and a marker are judged above; layout and size are not
	}
	if why := s.baseViolation(rel, codeExts[ext]); !why.Empty() {
		return why
	}
	if s.settings.EnforceConventions == nil || *s.settings.EnforceConventions {
		if why := conventionViolation(s.Root, abs, before, text); !why.Empty() {
			return M("protocol.convention", "why", why)
		}
	}
	if s.settings.EnforceArchitecture == nil || *s.settings.EnforceArchitecture {
		if why := archCheck(s.Root, s.workRoot(), rel, before, text); !why.Empty() {
			return M("protocol.architecture", "why", why)
		}
	}
	return s.sizeViolation(rel, ext, before, text)
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
