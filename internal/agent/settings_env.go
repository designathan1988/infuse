package agent

import (
	"sort"
	"strings"
)

// The environment variables of the settings' layers: Bash and the hooks get them, the model does not.

// settingsEnv lists the variables as KEY=value, in a stable order.
func (s *Session) settingsEnv() []string {
	out := make([]string, 0, len(s.settings.Env))
	for k, v := range s.settings.Env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// minMaskedValue is the shortest value worth masking: a one-letter value would rewrite ordinary text.
const minMaskedValue = 4

// maskSettingsEnv hides the variables' values in what a tool returns, so printing the environment or
// reading a file that holds them does not hand them to the model.
func (s *Session) maskSettingsEnv(out string) string {
	for k, v := range s.settings.Env {
		if len(v) >= minMaskedValue {
			out = strings.ReplaceAll(out, v, "[env:"+k+"]")
		}
	}
	return out
}
