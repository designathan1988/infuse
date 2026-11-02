package agent

// Destructive confirmation: commands that discard work (a data table) ask the user in every mode,
// bypass included. Only an explicit allow rule of the user's (Bash(rm -rf build:*)) skips the question;
// a plain "allow Bash", or a project file, does not.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"infuse/internal/data"
	"infuse/internal/filter"
	"regexp"
	"sync"
)

type destructiveRule struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Pattern string `json:"pattern"`
	re      *regexp.Regexp
}

var destructiveRules = sync.OnceValue(func() []destructiveRule {
	var rules []destructiveRule
	if err := json.Unmarshal(data.Read("destructive.json"), &rules); err != nil {
		panic("infuse: destructive.json: " + err.Error())
	}
	for i := range rules {
		rules[i].re = regexp.MustCompile(rules[i].Pattern)
	}
	return rules
})

// destructiveMatch finds the first rule a command breaks. The heredoc bodies are out: a document that
// mentions a command does not run it.
func destructiveMatch(command string) (destructiveRule, bool) {
	text := filter.MaskHeredocs(command)
	for _, r := range destructiveRules() {
		if r.re.MatchString(text) {
			return r, true
		}
	}
	return destructiveRule{}, false
}

// destructiveGate returns the decision after the check: "allow" once the user confirmed, or a refusal.
func (s *Session) destructiveGate(ctx context.Context, tool Tool, b Block, decision string) (string, string, bool) {
	if b.Name != "Bash" || decision == "deny" {
		return decision, "", false
	}
	command := bashCommand(b.Input)
	rule, hit := destructiveMatch(command)
	if !hit || s.patternDecision("Bash", b.Input) == "allow" {
		return decision, "", false
	}
	// "don't ask again" covers this exact command in this session: never the whole pattern
	key := fmt.Sprintf("destructive:%s:%x", rule.ID, sha256.Sum256([]byte(command)))
	s.Mu.Lock()
	silenced := s.allowAlways["sentinel:"+key]
	s.Mu.Unlock()
	if silenced {
		return "allow", "", false
	}
	why := M("destructive.ask", "what", rule.Label)
	Journal(s.Root, JournalEntry{Kind: "sentinel", Command: command, Detail: why.Render(s.uiLang())})
	if !s.askPermissionWhy(ctx, tool, b.Input, key, why) {
		return decision, s.say(b.ID, M("perm.user_refused", "why", why)), true
	}
	return "allow", "", false
}
