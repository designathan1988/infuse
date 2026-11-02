package agent

// Turn completion and failure.

func (s *Session) copyMessages() []Message {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

// donePayloadLocked is how the turn ended, as the interface keeps it: the accounts and the outcome.
func (s *Session) donePayloadLocked(reason string) map[string]any {
	return map[string]any{"reason": reason, "cost": s.Cost, "usage": s.Usage, "outcome": s.Outcome, "turnCost": s.turnCostLocked(),
		"turnCosts": append([]float64{}, s.TurnCosts...), "unpriced": append([]string{}, s.Unpriced...)}
}

func (s *Session) finish(reason string) {
	s.Mu.Lock()
	s.cancel = nil
	s.Working = false
	if reason == "" { // the turn went through its obligations; a cancelled one did not
		s.AuditPending = false
	}
	done := s.donePayloadLocked(reason)
	s.Mu.Unlock()
	s.save() // the terminal state is observed only after the document is durable
	s.emit("done", done)
	s.setState("idle")
	s.Mu.Lock()
	running := s.Mission != nil && s.Mission.State == "running"
	s.Mu.Unlock()
	if running {
		if reason == "" {
			go s.missionStep() // verdict, budget, and the next turn
		} else {
			s.endMission("parada", reason)
		}
	}
}

func (s *Session) fail(err error) {
	s.Mu.Lock()
	s.Working = false
	s.lastError = err.Error()
	s.cancel = nil
	s.Mu.Unlock()
	s.save()
	s.emit("error", err.Error())
	s.setState("error")
	s.endMission("parada", "error: "+err.Error())
}

// ---------------------------------------------------------------- compaction
