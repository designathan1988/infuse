package agent

import "strings"

// Fork starts a new session with this one's conversation up to message upTo (inclusive): the same
// project, model, provider, mode and session memory, to try another path from that point.
func (s *Session) Fork(upTo int) (*Session, error) {
	s.Mu.Lock()
	if upTo < 0 || upTo >= len(s.Messages) {
		s.Mu.Unlock()
		return nil, userError(s.uiLang(), M("fork.no_message"))
	}
	msgs := append([]Message{}, s.Messages[:upTo+1]...)
	// a reply that asked for tools without their results would be rejected by the provider: cut it
	for len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		pendingTools := false
		for _, b := range last.Content {
			if b.Type == "tool_use" {
				pendingTools = true
			}
		}
		if last.Role == "assistant" && pendingTools {
			msgs = msgs[:len(msgs)-1]
			continue
		}
		break
	}
	var starts []int
	for _, st := range s.TurnStarts {
		if st < len(msgs) {
			starts = append(starts, st)
		}
	}
	title, model, provider, mode, effort := s.Title, s.Model, s.Provider, s.Mode, s.Effort
	memory := append([]MemoryItem{}, s.Memory...)
	s.Mu.Unlock()

	n := newSession(s.Root)
	n.Mu.Lock()
	n.Messages, n.TurnStarts, n.Turn = msgs, starts, len(starts)
	n.Model, n.Provider, n.Mode, n.Effort, n.Memory = model, provider, mode, effort, memory
	n.Title = "↳ " + strings.TrimPrefix(title, "↳ ")
	n.Mu.Unlock()
	n.save()
	return n, nil
}

// TogglePin pins or unpins a message.
func (s *Session) TogglePin(i int) []int {
	s.Mu.Lock()
	out := s.PinnedMsgs[:0:0]
	found := false
	for _, p := range s.PinnedMsgs {
		if p == i {
			found = true
			continue
		}
		out = append(out, p)
	}
	if !found && i >= 0 && i < len(s.Messages) {
		out = append(out, i)
	}
	s.PinnedMsgs = out
	s.Mu.Unlock()
	s.save()
	return out
}
