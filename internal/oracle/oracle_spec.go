package oracle

// The executor: "done" is decided by running the oracle, never by the agent's word (SPEC §4.6).
//
// Criteria live in .infuse/oracle/checks.json, written by the user, readable by the agent, unreachable
// for it (the guard protects the oracle and the evidence). Only the user's "Ready?" runs the executor;
// no agent tool can. A run:
//   1. takes an immutable snapshot: a real copy of the working tree with a sha256 manifest, ignored and
//      dependency files included, sharing nothing with the live project (oracle_snapshot.go);
//   2. runs every check there, contained (oracle_contain.go), with the adapters (command, http, browser,
//      file, maxLines), an optional setup command first (e.g. installing dependencies);
//   3. stores evidence under .infuse/evidence/<run>/ (outputs, bodies, the snapshot id) and returns a
//      verdict: aprovado only when every check passes. "Not achieved, here is the evidence" is a
//      first-class result.

import (
	"errors"
	"fmt"
	"infuse/internal/browser"
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type OracleCheck struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // command | http | browser | file | maxLines
	Run         string `json:"run,omitempty"`
	URL         string `json:"url,omitempty"`
	Path        string `json:"path,omitempty"`
	Glob        string `json:"glob,omitempty"`
	Max         int    `json:"max,omitempty"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	Description string `json:"description,omitempty"`
	// Steps are the browser check's actions on the page, in order: click, type or press.
	Steps  []browser.ScenarioStep `json:"steps,omitempty"`
	Expect struct {
		Exit        *int   `json:"exit,omitempty"`
		Status      int    `json:"status,omitempty"`
		Contains    string `json:"contains,omitempty"`
		NotContains string `json:"notContains,omitempty"`
		Matches     string `json:"matches,omitempty"`
		Exists      *bool  `json:"exists,omitempty"`
		Selector    string `json:"selector,omitempty"` // browser: an element that must be on the page
		MaxMillis   int    `json:"maxMillis,omitempty"`
	} `json:"expect"`
}

type OracleSpec struct {
	store.Versioned
	Setup             string        `json:"setup,omitempty"`             // runs in the snapshot before the checks
	Server            string        `json:"server,omitempty"`            // HTTP and browser checks talk to this server, started in the snapshot
	ServerSec         int           `json:"serverTimeoutSec,omitempty"`  // how long to wait for it (default 60)
	RequireAcceptance bool          `json:"requireAcceptance,omitempty"` // a high-stakes delivery also needs the user's acceptance
	Checks            []OracleCheck `json:"checks"`
}

type CheckResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"` // the command, address, path or glob the check measured
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence,omitempty"` // file under the run's evidence folder
	Millis   int64  `json:"millis"`
	// Screenshot is the browser check's capture, under the run's evidence folder.
	Screenshot string `json:"screenshot,omitempty"`
	// Errored means the check could not be measured at all, as opposed to measured and failed.
	Errored bool `json:"errored,omitempty"`
}

// checkTarget is what a check measures, for the verdict the user reads.
func checkTarget(c OracleCheck) string {
	switch c.Kind {
	case "command":
		return c.Run
	case "http", "browser":
		return c.URL
	case "file":
		return c.Path
	case "maxLines":
		return c.Glob
	}
	return ""
}

type Verdict struct {
	Run          string        `json:"run"`
	Started      int64         `json:"started"`
	Finished     int64         `json:"finished"`
	Snapshot     string        `json:"snapshot"` // the base commit, when there is one, and the manifest's hash
	Verdict      string        `json:"verdict"`  // aprovado | reprovado | sem-criterios | erro
	Summary      string        `json:"summary"`
	Checks       []CheckResult `json:"checks"`
	Session      string        `json:"session,omitempty"`
	Turn         int           `json:"turn,omitempty"`         // the session turn this verdict belongs to
	SpecHash     string        `json:"specHash,omitempty"`     // sha256 of the criteria the run used
	Criteria     int           `json:"criteria,omitempty"`     // how many checks the criteria had
	Warnings     []string      `json:"warnings,omitempty"`     // limits of the run, stated with the verdict
	ServerPID    int           `json:"serverPid,omitempty"`    // the process the checks talked to, when the criteria declared one
	Project      string        `json:"project,omitempty"`      // which project the verdict was produced for
	Fingerprint  string        `json:"fingerprint,omitempty"`  // sha256 of the snapshot's manifest, stored as snapshot.txt
	Dependencies string        `json:"dependencies,omitempty"` // lockfiles + installed code the run measured
	Signature    string        `json:"signature,omitempty"`    // HMAC over every other field, key kept outside the project

	// SpecVerified is set when the verdict is read: the criteria stored with the run still hash to
	// the value the verdict recorded and the verdict's signature verifies, so neither was edited after.
	SpecVerified bool `json:"specVerified,omitempty"`
}

func SpecFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "oracle", "checks.json")
}
func EvidenceDir(root string) string { return filepath.Join(root, core.InfuseDirName, "evidence") }

func LoadOracleSpec(root string) (OracleSpec, error) {
	var s OracleSpec
	err := core.ReadJSON(SpecFile(root), &s)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	_, _ = s.Upgrade(nil)
	return s, nil
}

// LintSpec refuses a criterion that cannot be observed: every check needs a concrete target and a
// kind the executor knows, ids and targets are not duplicated, and a check that cannot fail is not
// a definition of done.
func LintSpec(s OracleSpec) error {
	ids := map[string]bool{}
	targets := map[string]bool{}
	for i, c := range s.Checks {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("check %d has no id", i+1)
		}
		if ids[c.ID] {
			return fmt.Errorf("check %s: duplicated id", c.ID)
		}
		ids[c.ID] = true
		switch c.Kind {
		case "command":
			if strings.TrimSpace(c.Run) == "" {
				return fmt.Errorf("check %s: command check without a command", c.ID)
			}
			if cannotFail(c.Run) {
				return fmt.Errorf("check %s: %q cannot fail; a criterion must be able to fail", c.ID, strings.TrimSpace(c.Run))
			}
		case "http":
			if strings.TrimSpace(c.URL) == "" {
				return fmt.Errorf("check %s: HTTP check without an address", c.ID)
			}
		case "browser":
			if err := lintBrowser(c); err != nil {
				return err
			}
		case "file":
			if strings.TrimSpace(c.Path) == "" {
				return fmt.Errorf("check %s: file check without a path", c.ID)
			}
		case "maxLines":
			if strings.TrimSpace(c.Glob) == "" || c.Max <= 0 {
				return fmt.Errorf("check %s: maxLines check needs a glob and a positive limit", c.ID)
			}
		default:
			return fmt.Errorf("check %s: unknown kind %q", c.ID, c.Kind)
		}
		key := c.Kind + "|" + checkTarget(c) + browserIdentity(c)
		if targets[key] {
			return fmt.Errorf("check %s: measures the same thing as another %s check (%s)", c.ID, c.Kind, checkTarget(c))
		}
		targets[key] = true
	}
	return nil
}

// SaveSpec writes the criteria stamped with the current schema version and anchors their hash in the
// user's own store, outside the project.
func SaveSpec(root string, s OracleSpec) error {
	if err := LintSpec(s); err != nil {
		return err
	}
	s.SchemaVersion = store.Version
	if err := core.WriteJSON(SpecFile(root), s); err != nil {
		return err
	}
	return SaveAnchor(root, s)
}

var executorMu sync.Map // root → *sync.Mutex: one run per project at a time

// RunOracle runs the executor for a project folder (the session's worktree when it has one) and
// stores the evidence under evidenceRoot (the project).
