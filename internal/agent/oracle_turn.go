package agent

// The verdict is part of the definition of done. When the user defined criteria, a turn that changed
// the project cannot end until the executor — which the agent cannot reach — has run them on the
// current tree and every check passed. The agent sees the failing checks and their evidence; its own
// word changes nothing.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"infuse/internal/oracle"
	"infuse/internal/workspace"
)

// criteriaRel is the criteria file as the turn audit names it.
const criteriaRel = ".infuse/oracle/checks.json"

// oracleObligation runs the user's criteria for this turn (at most once per tree state) and returns
// what still fails.
func (s *Session) oracleObligation(ctx context.Context) Msg {
	spec, err := LoadOracleSpec(s.Root)
	if err != nil { // an unreadable criteria file must not turn the executor off silently
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "oracle criteria unreadable: " + err.Error()})
		return M("turn.oracle_spec", "error", err.Error())
	}
	if len(spec.Checks) == 0 {
		// nothing to run, unless the user's criteria were removed: that is a refusal, not an absence
		if err := oracle.VerifyCriteria(s.Root, spec); err != nil {
			Journal(s.Root, JournalEntry{Kind: "gate", Detail: "oracle criteria removed: " + err.Error()})
			return M("turn.oracle_spec", "error", err.Error())
		}
		return Msg{}
	}
	key := s.oracleRunKey()
	s.Mu.Lock()
	cached, cachedKey := s.gate.oracleVerdict, s.gate.oracleKey
	s.Mu.Unlock()
	if cached == "aprovado" && cachedKey == key {
		return Msg{}
	}
	v, err := RunOracleTurn(ctx, s.Root, s.workRoot(), s.ID, s.Turn)
	s.acceptExecutorWrites(v.Run)
	if err != nil {
		return M("turn.oracle", "verdict", "error", "summary", err.Error(), "items", "")
	}
	s.Mu.Lock()
	s.gate.oracleKey, s.gate.oracleVerdict = key, v.Verdict
	s.lastOracleRun, s.lastOracleSum, s.lastOracleErr = v.Run, v.Summary, missionDetail(v.Checks)
	s.Mu.Unlock()
	if v.Verdict == "aprovado" {
		return Msg{}
	}
	return M("turn.oracle", "verdict", v.Verdict, "summary", v.Summary, "items", failingChecks(v.Checks))
}

// acceptExecutorWrites moves into the turn's baseline what this run of the executor wrote, and nothing
// else: its own evidence folder and criteria the user really saved. A criteria file the agent changed
// matches no anchor, so it stays a protected change however often the executor runs.
func (s *Session) acceptExecutorWrites(run string) {
	evidence := ".infuse/evidence/" + run + "/"
	workspace.RefreshTurnProtected(s.Root, s.ID, s.Turn, func(rel string) bool {
		return run != "" && strings.HasPrefix(rel, evidence) || isAnchoredCriteria(s.Root, rel)
	})
}

// isAnchoredCriteria reports whether rel is the criteria file holding exactly what the user saved.
func isAnchoredCriteria(root, rel string) bool {
	return rel == criteriaRel && oracle.CriteriaAnchored(root)
}

// protectedChanges lists the protected files changed since the turn started, minus criteria the user
// saved while it ran: the user editing them mid-turn is not the agent writing them.
func (s *Session) protectedChanges() []string {
	all := workspace.TurnProtectedChanges(s.Root, s.ID, s.Turn)
	out := all[:0]
	for _, rel := range all {
		if !isAnchoredCriteria(s.Root, rel) {
			out = append(out, rel)
		}
	}
	return out
}

// oracleRunKey identifies the tree the verdict belongs to: the turn, the criteria, the content of the
// files this turn touched and of the installed dependencies, which the checks run but no project-file
// audit sees. The same state reuses the verdict instead of re-running.
func (s *Session) oracleRunKey() string {
	b, _ := os.ReadFile(SpecFile(s.Root))
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%d|%s|%s|%s", s.Turn, hex.EncodeToString(sum[:8]), s.changeFingerprint(), oracle.DependencyFingerprint(s.workRoot()))
}

// changeFingerprint hashes the content of the files this turn changed. Modification times and sizes
// are not used: a timestamp restored with touch and same-size padding would replay a stale verdict.
func (s *Session) changeFingerprint() string {
	s.Mu.Lock()
	seen := make(map[string]bool, len(s.Checkpoints[s.Turn]))
	for _, c := range s.Checkpoints[s.Turn] {
		seen[c.Path] = true
	}
	s.Mu.Unlock()
	for _, ch := range s.ungatedChanges() {
		seen[ch.Path] = true
	}
	paths := make([]string, 0, len(seen))
	for rel := range seen {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, rel := range paths {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		f, err := os.Open(filepath.Join(s.workRoot(), filepath.FromSlash(rel)))
		if err != nil {
			h.Write([]byte("missing"))
			continue
		}
		_, _ = io.Copy(h, f)
		_ = f.Close()
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// failingChecks lists the checks that did not pass, with their detail.
func failingChecks(checks []CheckResult) string {
	var out []string
	for _, c := range checks {
		if !c.Passed {
			target := c.Target
			if target != "" {
				target = " (" + target + ")"
			}
			out = append(out, "· "+c.ID+target+": "+c.Detail)
		}
	}
	if len(out) > 12 {
		out = append(out[:12], "…")
	}
	return strings.Join(out, "\n")
}
