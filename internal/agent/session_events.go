package agent

import (
	"encoding/json"
	"time"
)

// Event fan-out, state snapshots and per-session selectors.

func (s *Session) subscribe() (chan Event, func()) {
	ch := make(chan Event, 256)
	s.Mu.Lock()
	s.subs[ch] = struct{}{}
	s.Mu.Unlock()
	return ch, func() {
		s.Mu.Lock()
		delete(s.subs, ch)
		s.Mu.Unlock()
	}
}

func (s *Session) emit(t string, data any) {
	s.Mu.Lock()
	s.seq++
	ev := Event{Type: t, Data: data, Seq: s.seq}
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
			// a subscriber that fell behind is disconnected rather than silently skipped: its client
			// reconnects and receives a fresh snapshot (pending permission and question included)
			close(ch)
			delete(s.subs, ch)
		}
	}
	s.Mu.Unlock()
}

func (s *Session) setState(st string) {
	s.Mu.Lock()
	s.State = st
	s.Mu.Unlock()
	s.emit("state", map[string]any{"state": st})
}

func (s *Session) snapshot() map[string]any {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return map[string]any{
		"id": s.ID, "root": s.Root, "title": s.Title, "model": s.Model, "state": s.State, "turn": s.Turn,
		"messages": s.Messages, "tasks": s.Tasks, "cwd": s.Cwd, "usage": s.Usage, "cost": s.Cost,
		"turnStarts": s.TurnStarts, "checkpoints": s.Checkpoints, "pendingPermission": s.pendingPerm,
		"pendingQuestion": s.pendingAsk, "error": s.lastError, "compactions": s.Compactions, "mode": s.mode(),
		"provider": s.providerID(), "effort": s.effort(), "archived": s.Archived, "worktree": s.Worktree, "branch": s.Branch,
		"workRoot": s.workRoot(), "plan": s.Plan, "mission": s.Mission.clone(), "pinnedMsgs": s.PinnedMsgs, "created": s.Created, "updated": s.Updated, "lastInput": s.LastInput,
		"procs": s.procListLocked(), "agents": s.Agents, "contextLimit": s.contextLimitLocked(), "diagnoses": s.Diagnoses,
	}
}

// mode, providerID, effort: the session's own choice, else the settings default.
func (s *Session) mode() string {
	if s.Mode != "" {
		return s.Mode
	}
	if s.settings.Mode == "" {
		return "default"
	}
	return s.settings.Mode
}

func (s *Session) providerID() string {
	if s.Provider != "" {
		return s.Provider
	}
	return s.settings.Provider
}

func (s *Session) effort() string {
	if s.Effort != "" {
		return s.Effort
	}
	return s.settings.Effort
}

// fastModel is the settings' fast model when the session's provider serves it, else the session's model
// (a fast model from another provider would fail on this connection).
func (s *Session) fastModel() string {
	fm := s.settings.FastModel
	if fm == "" {
		return s.Model
	}
	c := s.settings.Connections[s.providerID()]
	for _, m := range c.Models {
		if m == fm {
			return fm
		}
	}
	if s.providerID() == s.settings.Provider && len(c.Models) == 0 {
		return fm
	}
	return s.Model
}

// workRoot is where the agent's tools operate: the session's worktree, or the project.
func (s *Session) workRoot() string {
	if s.Worktree != "" {
		return s.Worktree
	}
	return s.Root
}

// thinkingBudget maps the effort level to a reasoning budget (0 = off).
func thinkingBudget(effort string, fallback int) int {
	switch effort {
	case "low":
		return 0
	case "medium":
		return 4000
	case "high":
		return 12000
	case "max":
		return 32000
	}
	return fallback
}

// ---------------------------------------------------------------- helpers used by tools

func (s *Session) isProtected(abs string) bool {
	_, ok := newGuard(s.Root, s.settings.ProtectedPaths).Protects(s.Root, abs)
	return ok
}

func bashCommand(input json.RawMessage) string {
	var in struct{ Command string }
	_ = json.Unmarshal(input, &in)
	return in.Command
}

func toolPath(input json.RawMessage) string {
	var in struct{ Path, FilePath string }
	_ = json.Unmarshal(input, &in)
	return firstNonEmpty(in.Path, in.FilePath)
}

func (s *Session) markRead(abs string) { s.Mu.Lock(); s.readFiles[abs] = true; s.Mu.Unlock() }
func (s *Session) wasRead(abs string) bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.readFiles[abs]
}
func (s *Session) setCwd(abs string) { s.Mu.Lock(); s.Cwd = abs; s.Mu.Unlock(); s.emit("cwd", abs) }
func (s *Session) setTasks(t []TaskItem) {
	s.Mu.Lock()
	s.Tasks = t
	s.Mu.Unlock()
	s.emit("tasks", t)
	s.save()
}

func (s *Session) scheduleMapRefresh() {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.mapTimer != nil {
		s.mapTimer.Stop()
	}
	root := s.Root
	s.mapTimer = time.AfterFunc(400*time.Millisecond, func() { WriteMap(root, BuildMap(root)); s.emit("map", nil) })
}
