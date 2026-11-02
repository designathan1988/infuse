package agent

// Applying the user's behaviour policy to a running session: settings overrides and prohibitions.

import (
	"fmt"
	"strings"

	"infuse/internal/config"
	"infuse/internal/policy"
)

const policyBanQuote = "project policy" // marks the prohibitions that come from the Rules document

// refreshPolicy reloads the global and project policy before a turn. A corrupt policy is fail-closed.
func (s *Session) refreshPolicy() {
	global, project, err := policy.LoadScopes(s.Root)
	if err != nil {
		s.Mu.Lock()
		s.policyErr = err
		s.Mu.Unlock()
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "behaviour policy unreadable: " + err.Error()})
		s.emitMsg("gate", M("policy.invalid", "error", err.Error()))
		return
	}
	s.Mu.Lock()
	s.policyErr = nil
	s.settings = config.EffectiveSettings(s.Root) // reset, then overlay the policy
	policy.ApplySettings(&s.settings, global, project)
	s.Mu.Unlock()
	s.applyPolicyBans(policy.ProhibitionsFrom(global, project))
}

// applyPolicyBans replaces the prohibitions that come from the policy, keeping the user's own session
// bans and their lifted entries untouched.
func (s *Session) applyPolicyBans(p policy.Prohibitions) {
	old := s.Bans()
	var list []Ban
	for _, b := range old {
		if b.Quote != policyBanQuote {
			list = append(list, b)
		}
	}
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			list = append(list, Ban{Kind: kind, Value: value, Quote: policyBanQuote, Policy: true})
		}
	}
	for _, v := range p.Commands {
		add("command", v)
	}
	for _, v := range p.Paths {
		add("path", v)
	}
	for _, v := range p.Tools {
		if toolNames[v] {
			add("tool", v)
		}
	}
	for _, v := range p.Actions {
		add("action", v)
	}
	if fmt.Sprint(old) == fmt.Sprint(list) {
		return
	}
	s.setBans(list)
}
