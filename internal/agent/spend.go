package agent

// The accounts of a session. Every model response of every kind (the loop, subagents, the compile steps,
// compaction, side questions) goes through spend, so the session's total, the turn's total, the day's
// ledger and the daily limit can never disagree. The limit warns first, then stops.

import (
	"fmt"
	"time"
)

// dailyWarnShare is how far into the daily limit the warning comes.
const dailyWarnShare = 0.8

// spend records one model response and returns its cost.
func (s *Session) spend(model string, u Usage) float64 {
	if u == (Usage{}) {
		return 0
	}
	cost := s.settings.CostOf(model, u)
	provider := s.providerID()
	priced := s.settings.Priced(provider, model)
	firstUnpriced := false
	s.Mu.Lock()
	s.Cost += cost
	s.Usage.Input += u.Input
	s.Usage.Output += u.Output
	s.Usage.CacheRead += u.CacheRead
	s.Usage.CacheWrite += u.CacheWrite
	turn := max(s.Turn, 1)
	for len(s.TurnCosts) < turn {
		s.TurnCosts = append(s.TurnCosts, 0)
	}
	s.TurnCosts[turn-1] += cost
	if !priced && !contains(s.Unpriced, model) {
		s.Unpriced = append(s.Unpriced, model)
		firstUnpriced = true
	}
	s.Mu.Unlock()
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: provider, Model: model, Input: u.Input, Output: u.Output, Cache: u.CacheRead + u.CacheWrite, Cost: cost, Unpriced: !priced})
	if firstUnpriced {
		s.emitMsg("gate", M("usage.unpriced", "model", model))
	}
	s.warnDailyLimit()
	return cost
}

// overDailyLimit is the stop every model call checks first: the day's total across every session.
func (s *Session) overDailyLimit() (Msg, bool) {
	lim := s.settings.DailyLimitUSD
	if lim <= 0 || s.unmeteredProvider() {
		return Msg{}, false
	}
	if spent := todaySpend(); spent >= lim {
		return M("session.daily_limit", "spent", usd(spent), "limit", usd(lim)), true
	}
	return Msg{}, false
}

// unmeteredProvider reports a local or subscription-backed provider. Claude Code Desktop, Codex,
// Ollama and the demonstration provider are not billed per token by infuse; applying the paid-API
// daily limit to them stops a local session for a cost that does not exist.
func (s *Session) unmeteredProvider() bool {
	id := s.Provider
	if id == "" {
		id = s.settings.Provider
	}
	kind := s.settings.Connections[id].Kind
	if kind == "" {
		kind = id
	}
	switch kind {
	case "mock", "claudedesk", "codexdesk", "ollama":
		return true
	}
	return false
}

// warnDailyLimit tells the user once a day, per session, that the limit is near.
func (s *Session) warnDailyLimit() {
	lim := s.settings.DailyLimitUSD
	if lim <= 0 || s.unmeteredProvider() {
		return
	}
	spent := todaySpend()
	if spent < lim*dailyWarnShare || spent >= lim {
		return
	}
	day := time.Now().Format("2006-01-02")
	s.Mu.Lock()
	warned := s.warnedDay == day
	s.warnedDay = day
	s.Mu.Unlock()
	if !warned {
		s.emitMsg("gate", M("session.daily_warning", "spent", usd(spent), "limit", usd(lim)))
	}
}

func usd(v float64) string { return fmt.Sprintf("%.2f", v) }

// turnCostLocked is what the current turn has cost so far.
func (s *Session) turnCostLocked() float64 {
	if i := max(s.Turn, 1) - 1; i < len(s.TurnCosts) {
		return s.TurnCosts[i]
	}
	return 0
}
