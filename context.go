package main

// The working context of a session: the model's own window, or less when the user set less.

func (s *Session) contextLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contextLimitLocked()
}

func (s *Session) contextLimitLocked() int {
	model := s.Model
	if model == "" {
		model = s.settings.Model
	}
	window := contextWindow(model)
	if lim := s.settings.ContextLimit; lim > 0 && lim < window {
		return lim
	}
	return window
}
