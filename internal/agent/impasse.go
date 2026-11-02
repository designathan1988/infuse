package agent

// An impasse outside a mission: the agent says it cannot go on. It is recorded with the session, the
// user reads the reason, what was tried and what is needed, and the turn ends as an impasse: not done,
// and not sent back to its obligations. The next turn still audits what this one left behind.

import "strings"

// declareImpasse records the impasse of the current turn.
func (s *Session) declareImpasse(tc *ToolContext, im Impasse) string {
	s.closeOpenTasks(M("impasse.task_note").Render(s.uiLang()))
	s.Mu.Lock()
	s.Impasse, s.impasseTurn = &im, s.Turn
	s.Mu.Unlock()
	s.setOutcome("impasse", M("impasse.declared", "reason", im.Reason, "tried", strings.Join(im.Tried, "; "), "needs", im.Needs))
	s.emit("impasse", im)
	s.save()
	return tc.Say(M("impasse.turn_recorded"))
}

// impassed reports whether the current turn ended at an impasse.
func (s *Session) impassed() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.impasseTurn > 0 && s.impasseTurn == s.Turn
}
