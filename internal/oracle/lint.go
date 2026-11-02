package oracle

// Lint helpers: deterministic refusals for criteria that look like criteria but cannot fail.

import (
	"fmt"
	"strings"
)

// documentRoots are always on a page: asserting them would pass on an empty one.
var documentRoots = map[string]bool{"html": true, "body": true, "head": true, ":root": true, "*": true}

// lintBrowser refuses a browser check without a real address, a valid script and something that can
// fail: opening a page and asserting nothing measures nothing.
func lintBrowser(c OracleCheck) error {
	if u := strings.TrimSpace(c.URL); !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("check %s: browser check needs an http or https address", c.ID)
	}
	for i, s := range c.Steps {
		switch s.Action {
		case "click", "type":
			if strings.TrimSpace(s.Selector) == "" {
				return fmt.Errorf("check %s: step %d (%s) has no selector", c.ID, i+1, s.Action)
			}
		case "press":
			if strings.TrimSpace(s.Text) == "" {
				return fmt.Errorf("check %s: step %d (press) has no key", c.ID, i+1)
			}
		default:
			return fmt.Errorf("check %s: step %d has unknown action %q", c.ID, i+1, s.Action)
		}
	}
	x := c.Expect
	if documentRoots[strings.ToLower(strings.TrimSpace(x.Selector))] {
		return fmt.Errorf("check %s: %q is on every page; a criterion must be able to fail", c.ID, x.Selector)
	}
	if x.Selector == "" && x.Contains == "" && x.NotContains == "" && x.Matches == "" {
		return fmt.Errorf("check %s: browser check needs an expected selector or text", c.ID)
	}
	return nil
}

// cannotFail reports whether a check command always exits zero by construction: it only writes to
// the void or into its own arguments. Such a "criterion" passes no matter what the project does.
func cannotFail(run string) bool {
	fields := strings.Fields(strings.TrimSpace(run))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "true", ":":
		return true
	case "echo", "printf":
		// an echo that only writes its own arguments leaves nothing for the executor to observe
		return !strings.ContainsAny(run, "$(`>|")
	}
	return false
}
