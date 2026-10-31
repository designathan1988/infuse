package oracle

// The executor: "done" is decided by running the oracle, never by the agent's word (SPEC §4.6).
//
// Criteria live in .infuse/oracle/checks.json, written by the user, readable by the agent, unreachable
// for it (the guard protects the oracle and the evidence). Only the user's "Pronto?" runs the executor;
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
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
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
	Setup  string        `json:"setup,omitempty"` // runs in the snapshot before the checks
	Checks []OracleCheck `json:"checks"`
}

type CheckResult struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Passed   bool   `json:"passed"`
	Detail   string `json:"detail"`
	Evidence string `json:"evidence,omitempty"` // file under the run's evidence folder
	Millis   int64  `json:"millis"`
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

// SaveSpec writes the criteria stamped with the current schema version.
func SaveSpec(root string, s OracleSpec) error {
	s.SchemaVersion = store.Version
	return core.WriteJSON(SpecFile(root), s)
}

var executorMu sync.Map // root → *sync.Mutex: one run per project at a time

// RunOracle runs the executor for a project folder (the session's worktree when it has one) and
// stores the evidence under evidenceRoot (the project).
