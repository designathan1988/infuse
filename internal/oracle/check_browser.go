package oracle

// The browser adapter: SPEC's "web via browser". It opens an address of the snapshot's server in a
// private headless browser, performs the declared steps, and asserts on what the page shows, with a
// screenshot as evidence. Without a browser the check cannot be measured, and that is an error of the
// run: never a pass, and never a failure charged to the project.

import (
	"context"
	"errors"
	"fmt"
	"infuse/internal/browser"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runScenario is the browser behind the check.
var runScenario = browser.RunScenario

const browserCheckTimeout = 60 * time.Second

func checkBrowser(ctx context.Context, evDir, safe string, c OracleCheck) CheckResult {
	r := CheckResult{ID: c.ID, Kind: c.Kind}
	timeout := browserCheckTimeout
	if c.TimeoutSec > 0 {
		timeout = time.Duration(c.TimeoutSec) * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	profile, err := os.MkdirTemp("", "b-")
	if err != nil {
		r.Errored, r.Detail = true, uiText("oracle.browserFailed", "error", err.Error())
		return r
	}
	defer os.RemoveAll(profile)
	res, err := runScenario(cctx, browser.Scenario{URL: c.URL, Steps: c.Steps, Selector: c.Expect.Selector, Profile: profile})
	if errors.Is(err, browser.ErrNoBrowser) {
		r.Errored, r.Detail = true, uiText("oracle.noBrowser")
		if err != browser.ErrNoBrowser { // one was found and would not start: say why
			r.Detail = uiText("oracle.browserFailed", "error", err.Error())
		}
		return r
	}
	writeBrowserEvidence(&r, evDir, safe, c, res, err)
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	ok, why := browserAssert(c, res)
	r.Passed, r.Detail = ok, core.FirstNonEmpty(why, fmt.Sprintf("HTTP %d", res.Status))
	return r
}

// browserAssert applies the expectation to the final page.
func browserAssert(c OracleCheck, res browser.ScenarioResult) (bool, string) {
	want := c.Expect.Status
	if want == 0 {
		want = 200
	}
	if res.Status != want {
		return false, fmt.Sprintf("HTTP %d (expected %d)", res.Status, want)
	}
	if c.Expect.Selector != "" && !res.Found {
		return false, fmt.Sprintf("element %q is not on the page", c.Expect.Selector)
	}
	return textOK(c, res.Text)
}

func writeBrowserEvidence(r *CheckResult, evDir, safe string, c OracleCheck, res browser.ScenarioResult, runErr error) {
	if len(res.Screenshot) > 0 {
		name := safe + ".png"
		if os.WriteFile(filepath.Join(evDir, name), res.Screenshot, 0o644) == nil {
			r.Screenshot = name
		}
	}
	lines := append([]string{"GET " + c.URL, fmt.Sprintf("HTTP %d", res.Status), "final address: " + res.FinalURL}, res.Log...)
	if runErr != nil {
		lines = append(lines, "error: "+runErr.Error())
	}
	writeEvidence(r, evDir, safe+".browser.txt", strings.Join(lines, "\n")+"\n\n"+res.Text+"\n")
}

// browserIdentity separates browser checks of one address: their steps and assertions are what they
// measure. Other kinds are told apart by the target alone.
func browserIdentity(c OracleCheck) string {
	if c.Kind != "browser" {
		return ""
	}
	return fmt.Sprint(c.Steps, c.Expect.Selector, c.Expect.Contains, c.Expect.NotContains, c.Expect.Matches, c.Expect.Status)
}
