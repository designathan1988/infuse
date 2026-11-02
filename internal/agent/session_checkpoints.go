package agent

import (
	"errors"
	"fmt"
	"infuse/internal/workspace"
	"os"
	"path/filepath"
	"time"
)

// Per-turn checkpoints, revert and map refresh.

func (s *Session) checkpointBeforeWrite(abs string) {
	rel := relTo(s.workRoot(), abs) // relative to where the tools work: a worktree's files stay inside it
	s.Mu.Lock()
	for _, c := range s.Checkpoints[s.Turn] {
		if c.Path == rel {
			s.Mu.Unlock()
			return
		}
	}
	cf := CheckpointFile{Path: rel, New: !exists(abs)}
	if !cf.New {
		dst := filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(s.Turn), filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if b, err := os.ReadFile(abs); err == nil {
			_ = os.WriteFile(dst, b, 0o644)
		}
	}
	s.Checkpoints[s.Turn] = append(s.Checkpoints[s.Turn], cf)
	s.Mu.Unlock()
	s.emit("checkpoint", map[string]any{"turn": s.Turn, "path": rel})
}

// Revert restores files to how they were before turn `toTurn` and truncates the conversation there.
func (s *Session) Revert(toTurn int) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.State != "idle" && s.State != "error" {
		return errors.New(M("session.stop_first_revert").Render(s.uiLang()))
	}
	for t := s.Turn; t >= toTurn; t-- {
		s.restoreTurnLocked(t)
	}
	if toTurn-1 < len(s.TurnStarts) && toTurn >= 1 {
		s.Messages = s.Messages[:s.TurnStarts[toTurn-1]]
		s.TurnStarts = s.TurnStarts[:toTurn-1]
	}
	s.Turn = toTurn - 1
	Journal(s.Root, JournalEntry{Kind: "revert", Detail: fmt.Sprintf("session %s reverted to turn %d", s.ID, toTurn)})
	s.saveLocked()
	s.scheduleMapRefreshLocked()
	return nil
}

// restoreTurnLocked puts the files back as they were when turn t started. The per-write checkpoints
// go first and only cover what the tree snapshot skips (large and build files); the snapshot is taken
// before anything in the turn ran, so it has the last word: a checkpoint is taken at the first write,
// possibly after a shell command already changed or deleted the file.
func (s *Session) restoreTurnLocked(t int) {
	files := s.Checkpoints[t]
	for i := len(files) - 1; i >= 0; i-- {
		c := files[i]
		abs := filepath.Join(s.workRoot(), filepath.FromSlash(c.Path))
		if c.New {
			_ = os.Remove(abs)
			continue
		}
		src := filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t), filepath.FromSlash(c.Path))
		if b, err := os.ReadFile(src); err == nil {
			_ = os.MkdirAll(filepath.Dir(abs), 0o755)
			_ = os.WriteFile(abs, b, 0o644)
		}
	}
	workspace.RestoreTurn(s.Root, s.workRoot(), s.ID, t)
	_ = os.RemoveAll(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t)))
	delete(s.Checkpoints, t)
}

func (s *Session) scheduleMapRefreshLocked() {
	root := s.Root
	time.AfterFunc(200*time.Millisecond, func() { WriteMap(root, BuildMap(root)); s.emit("map", nil); s.emit("session", s.snapshot()) })
}

// ---------------------------------------------------------------- permissions
