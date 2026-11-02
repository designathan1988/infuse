package agent

// Pattern rules: entries of the permissions setting written Tool(pattern), next to the plain tool
// entries and merged the same way across the layers (a project can only tighten them):
//   Bash(git status:*)  a command prefix (":*" = anything after it); without ":*", the whole command
//   Write(src/**)       a path glob, for the tools that take a path
//   WebFetch(*.go.dev)  the address's host, as a glob
// The strictest rule that matches decides. A command line is judged segment by segment: one denied
// segment denies the line, and "allow" needs every segment allowed.

import (
	"encoding/json"
	"infuse/internal/filter"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

type permRule struct {
	tool, pattern, decision string
}

var rePermRule = regexp.MustCompile(`^([A-Za-z_][\w-]*)\((.+)\)$`)

// permRules are the pattern entries of the settings.
func (s *Session) permRules(tool string) []permRule {
	var out []permRule
	for k, v := range s.settings.Permissions {
		if m := rePermRule.FindStringSubmatch(k); m != nil && m[1] == tool {
			out = append(out, permRule{m[1], m[2], v})
		}
	}
	return out
}

// reSubstitution finds a command inside a command: its text is not what a prefix rule looked at.
var reSubstitution = regexp.MustCompile("\\$\\(|`|<\\(|>\\(")

// patternDecision is the verdict of the pattern rules on a call: deny, ask, allow, or "" when none
// applies. Allow is never the answer for a line whose subcommands no rule saw.
func (s *Session) patternDecision(tool string, input json.RawMessage) string {
	rules := s.permRules(tool)
	if len(rules) == 0 {
		return ""
	}
	if tool == "Bash" {
		return bashDecision(rules, bashCommand(input))
	}
	subject := s.ruleSubject(tool, input)
	if subject == "" {
		return ""
	}
	return strictest(rules, func(r permRule) bool { return globMatches(r.pattern, subject) })
}

// bashDecision judges each segment; allow survives only when every segment has an allow rule.
func bashDecision(rules []permRule, command string) string {
	segments := filter.CommandSegments(command)
	verdict := ""
	allowed := len(segments) > 0 && !reSubstitution.MatchString(command)
	for _, seg := range segments {
		d := strictest(rules, func(r permRule) bool { return commandMatches(r.pattern, seg) })
		switch d {
		case "deny":
			return "deny"
		case "ask":
			verdict = "ask"
		}
		allowed = allowed && d == "allow"
	}
	if verdict == "" && allowed {
		return "allow"
	}
	return verdict
}

// strictest applies the matching rules: deny over ask over allow.
func strictest(rules []permRule, matches func(permRule) bool) string {
	best := ""
	rank := map[string]int{"": 0, "allow": 1, "ask": 2, "deny": 3}
	for _, r := range rules {
		if matches(r) && rank[r.decision] > rank[best] {
			best = r.decision
		}
	}
	return best
}

// commandMatches: "git status:*" is the prefix, as a whole word; anything else is the exact command.
func commandMatches(pattern, command string) bool {
	command = strings.Join(strings.Fields(command), " ")
	if p, ok := strings.CutSuffix(pattern, ":*"); ok {
		return command == p || strings.HasPrefix(command, p+" ")
	}
	return command == strings.Join(strings.Fields(pattern), " ")
}

func globMatches(pattern, subject string) bool {
	re := globToRegex(pattern)
	return re != nil && re.MatchString(subject)
}

// ruleSubject is what a rule of this tool is matched against: the path relative to the project, or
// the address's host.
func (s *Session) ruleSubject(tool string, input json.RawMessage) string {
	if tool == "WebFetch" {
		var in struct{ URL string }
		_ = json.Unmarshal(input, &in)
		if u, err := url.Parse(in.URL); err == nil {
			return u.Hostname()
		}
		return ""
	}
	p := toolPath(input)
	if p == "" {
		return ""
	}
	abs := FromMsysPath(p)
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.Cwd, abs)
	}
	return relTo(s.workRoot(), abs)
}

// alwaysAllowed reports the session's "always" answers: a whole tool, or for Bash the prefix of every
// segment of the line.
func (s *Session) alwaysAllowed(name string, input json.RawMessage) bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.allowAlways[name] {
		return true
	}
	if name != "Bash" {
		return false
	}
	command := bashCommand(input)
	segments := filter.CommandSegments(command)
	if len(segments) == 0 || reSubstitution.MatchString(command) {
		return false
	}
	for _, seg := range segments {
		if !s.allowAlways[alwaysKey(commandPrefix(seg))] {
			return false
		}
	}
	return true
}

func alwaysKey(prefix string) string { return "Bash:" + prefix }

// prefixedCommands are the programs named with their subcommand: "git" alone would allow too much.
var prefixedCommands = map[string]bool{"git": true, "npm": true, "go": true, "docker": true, "pnpm": true, "yarn": true}

// commandPrefix is how an "always" answer names a command.
func commandPrefix(command string) string {
	f := strings.Fields(command)
	if len(f) == 0 {
		return ""
	}
	if len(f) > 1 && prefixedCommands[f[0]] {
		return f[0] + " " + f[1]
	}
	return f[0]
}

// bashPrefix is the prefix "always" remembers: the line's first command, the one the question shows.
func bashPrefix(tool string, input json.RawMessage) string {
	if tool != "Bash" {
		return ""
	}
	segments := filter.CommandSegments(bashCommand(input))
	if len(segments) == 0 {
		return ""
	}
	return commandPrefix(segments[0])
}
