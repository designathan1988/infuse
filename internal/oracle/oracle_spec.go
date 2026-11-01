package oracle

// The executor: "done" is decided by running the oracle, never by the agent's word (SPEC §4.6).
//
// Criteria live in .infuse/oracle/checks.json, written by the user, readable by the agent, unreachable
// for it (the guard protects the oracle and the evidence). Only the user's "Ready?" runs the executor;
// no agent tool can. A run:
//   1. takes an immutable snapshot: a git commit of the working tree (tracked + untracked, through a
//      temporary index, the user's index untouched) checked out in a detached worktree; projects
//      without git get a copy of their files and a sha256 manifest;
//   2. runs every check there with the adapters (command, http, file, maxLines), an optional setup
//      command first (e.g. installing dependencies);
//   3. stores evidence under .infuse/evidence/<run>/ (outputs, bodies, the snapshot id) and returns a
//      verdict: aprovado only when every check passes. "Not achieved, here is the evidence" is a
//      first-class result.

import (
	"errors"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type OracleCheck struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"` // command | http | file | maxLines
	Run         string `json:"run,omitempty"`
	URL         string `json:"url,omitempty"`
	Path        string `json:"path,omitempty"`
	Glob        string `json:"glob,omitempty"`
	Max         int    `json:"max,omitempty"`
	TimeoutSec  int    `json:"timeoutSec,omitempty"`
	Description string `json:"description,omitempty"`
	Expect      struct {
		Exit        *int   `json:"exit,omitempty"`
		Status      int    `json:"status,omitempty"`
		Contains    string `json:"contains,omitempty"`
		NotContains string `json:"notContains,omitempty"`
		Matches     string `json:"matches,omitempty"`
		Exists      *bool  `json:"exists,omitempty"`
		MaxMillis   int    `json:"maxMillis,omitempty"`
	} `json:"expect"`
}

type OracleSpec struct {
	store.Versioned
	Setup     string        `json:"setup,omitempty"`            // runs in the snapshot before the checks
	Server    string        `json:"server,omitempty"`           // HTTP checks talk to this server, started in the snapshot
	ServerSec int           `json:"serverTimeoutSec,omitempty"` // how long to wait for it (default 60)
	Checks    []OracleCheck `json:"checks"`
}

type CheckResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Target   string `json:"target,omitempty"` // the command, address, path or glob the check measured
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence,omitempty"` // file under the run's evidence folder
	Millis   int64  `json:"millis"`
}

// checkTarget is what a check measures, for the verdict the user reads.
func checkTarget(c OracleCheck) string {
	switch c.Kind {
	case "command":
		return c.Run
	case "http":
		return c.URL
	case "file":
		return c.Path
	case "maxLines":
		return c.Glob
	}
	return ""
}

type Verdict struct {
	Run      string        `json:"run"`
	Started  int64         `json:"started"`
	Finished int64         `json:"finished"`
	Snapshot string        `json:"snapshot"` // git commit or sha256 of the file manifest
	Verdict  string        `json:"verdict"`  // aprovado | reprovado | sem-criterios | erro
	Summary  string        `json:"summary"`
	Checks   []CheckResult `json:"checks"`
	Session  string        `json:"session,omitempty"`
	Turn     int           `json:"turn,omitempty"`     // the session turn this verdict belongs to
	SpecHash string        `json:"specHash,omitempty"` // sha256 of the criteria the run used
	Criteria int           `json:"criteria,omitempty"` // how many checks the criteria had
	Warnings []string      `json:"warnings,omitempty"` // limits of the run, stated with the verdict
	ServerPID int          `json:"serverPid,omitempty"` // the process the checks talked to, when the criteria declared one
	Signature string       `json:"signature,omitempty"` // HMAC over every other field, key kept outside the project

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
// kind the executor knows. A vague statement is not a definition of done.
func LintSpec(s OracleSpec) error {
	for i, c := range s.Checks {
		if strings.TrimSpace(c.ID) == "" {
			return fmt.Errorf("check %d has no id", i+1)
		}
		switch c.Kind {
		case "command":
			if strings.TrimSpace(c.Run) == "" {
				return fmt.Errorf("check %s: command check without a command", c.ID)
			}
		case "http":
			if strings.TrimSpace(c.URL) == "" {
				return fmt.Errorf("check %s: HTTP check without an address", c.ID)
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
