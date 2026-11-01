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
)

// oracleObligation runs the user's criteria for this turn (at most once per tree state) and returns
// what still fails.
func (s *Session) oracleObligation(ctx context.Context) Msg {
	spec, err := LoadOracleSpec(s.Root)
	if err != nil { // an unreadable criteria file must not turn the executor off silently
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "oracle criteria unreadable: " + err.Error()})
		return M("turn.oracle_spec", "error", err.Error())
	}
	if len(spec.Checks) == 0 {
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
	RefreshTurnProtected(s.Root, s.ID, s.Turn) // the evidence is the executor's, not the agent's
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

// oracleRunKey identifies the tree the verdict belongs to: the turn, the criteria and the newest
// change of the files this turn touched. The same state reuses the verdict instead of re-running.
func (s *Session) oracleRunKey() string {
	b, _ := os.ReadFile(SpecFile(s.Root))
	sum := sha256.Sum256(b)
	return fmt.Sprintf("%d|%s|%s", s.Turn, hex.EncodeToString(sum[:8]), s.changeFingerprint())
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
