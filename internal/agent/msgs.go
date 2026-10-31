package agent

// Session-bound message recording. The catalog itself lives in internal/core; a message is recorded
// with the tool call (or the event) it belongs to, so the user's version is built from data, never
// from parsing words.

// say records a message produced for a tool call and returns the model's version.
func (s *Session) say(callID string, m Msg) string {
	if m.Empty() {
		return ""
	}
	s.Mu.Lock()
	if s.callMsgs == nil {
		s.callMsgs = map[string][]Msg{}
	}
	s.callMsgs[callID] = append(s.callMsgs[callID], m)
	s.Mu.Unlock()
	return m.String()
}

// Say is say for a tool: the message belongs to the call the tool is serving.
func (tc *ToolContext) Say(m Msg) string {
	if tc.S == nil {
		return m.String()
	}
	return tc.S.say(tc.CallID, m)
}

// takeCallMsgs returns, and forgets, the messages recorded for a call.
func (s *Session) takeCallMsgs(callID string) []Msg {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	ms := s.callMsgs[callID]
	delete(s.callMsgs, callID)
	return ms
}

// emitMsg sends a message to the user as an event (a gate, a notice) in both versions.
func (s *Session) emitMsg(kind string, m Msg) {
	s.emit(kind, map[string]any{"message": m.String(), "display": m.Render(s.uiLang()), "msg": m})
}

// textBlockLocked is textBlock for a caller that holds s.Mu.
func (s *Session) textBlockLocked(m Msg) Block {
	b := Block{Type: "text", Text: m.String()}
	if d := m.Render(s.uiLang()); d != b.Text {
		b.Display = d
	}
	return b
}

// textBlock is a note infuse adds to the conversation: English for the model, the user's language on
// screen.
func (s *Session) textBlock(m Msg) Block {
	b := Block{Type: "text", Text: m.String()}
	if d := m.Render(s.uiLang()); d != b.Text {
		b.Display = d
	}
	return b
}
