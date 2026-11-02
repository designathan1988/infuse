package agent

// The turn contract as a gate: work with more than one step does not start until the user confirms the
// agent's restatement (goal + verifiable criteria) for THIS request. A new request needs a new one.

func (s *Session) contractGate() Msg {
	if s.settings.RequireContract != nil && !*s.settings.RequireContract {
		return Msg{}
	}
	s.Mu.Lock()
	steps, needed := s.gate.planSteps, s.gate.contractNeeded
	contract := s.Contract
	s.Mu.Unlock()
	if !needed && steps < 2 { // only this turn's plan, not tasks left from an earlier request
		return Msg{}
	}
	if request := s.turnRequest(); request != "" && contract != nil && contract.Request == request {
		return Msg{}
	}
	return M("contract.required")
}
