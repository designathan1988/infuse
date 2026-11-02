package agent

// Detectors for the vices no single write shows: scope cut after a failure, a feature scattered into
// folders nothing connects, and code written that nothing reaches. Syntax and the map, never a model.

import (
	"fmt"
	"infuse/internal/projectmap"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// reDisabled finds behavior switched off in code.
var reDisabled = regexp.MustCompile(`(?m)\bif\s*\(?\s*(?:false|0|False)\s*\)?\s*[{:]|\b(?:enabled|active)\s*[:=]\s*(?:false|False)\b|\bdisabled\s*[:=]\s*(?:true|True)\b|#\[ignore\]|^\s*return\s*;?\s*(?://|#).*$`)

// scopeCut: in a turn where a test run failed, a production edit that removes exports or switches
// behavior off is the failure being made to go away, not fixed.
func (s *Session) scopeCut(rel, before, after string) Msg {
	if before == "" || !codeExts[strings.ToLower(filepath.Ext(rel))] || isTestPath(rel) || !s.testFailedThisTurn() {
		return Msg{}
	}
	if gone := removedExports(rel, before, after); len(gone) > 0 {
		return M("vice.scope_cut", "file", rel, "what", strings.Join(firstN(gone, 6), ", "))
	}
	for _, line := range addedLines(before, after) {
		if reDisabled.MatchString(line) {
			return M("vice.scope_cut", "file", rel, "what", truncate(strings.TrimSpace(line), 120))
		}
	}
	return Msg{}
}

func (s *Session) testFailedThisTurn() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for _, r := range s.gate.ran {
		if !r.ok && reTestRunner.MatchString(r.cmd) {
			return true
		}
	}
	return false
}

// removedExports lists what the file offered before and no longer does; a rename leaves one new name
// for each it took away, so only the surplus counts.
func removedExports(rel, before, after string) []string {
	now := map[string]bool{}
	for _, e := range projectmap.ExportsOf(rel, after) {
		now[e] = true
	}
	had := map[string]bool{}
	var gone []string
	for _, e := range projectmap.ExportsOf(rel, before) {
		had[e] = true
		if !now[e] {
			gone = append(gone, e)
		}
	}
	added := 0
	for e := range now {
		if !had[e] {
			added++
		}
	}
	if len(gone) <= added {
		return nil
	}
	return gone
}

// structureObligation looks at the turn's code through a fresh map, once per turn: exports the turn
// added that nothing references, and new files that scatter a feature. It sends the agent back one
// time; a second end is accepted, because only the user can say an export is the product's surface.
func (s *Session) structureObligation(changed []CheckpointFile) Msg {
	s.Mu.Lock()
	asked := s.gate.structureAsked
	s.Mu.Unlock()
	if asked || !s.changedCode(changed) {
		return Msg{}
	}
	m := BuildMap(s.workRoot())
	var items []Msg
	if d := s.deadExports(m, changed); d != "" {
		items = append(items, M("vice.dead_code", "items", d))
	}
	if sp := scatteredFiles(m, changed); sp != "" {
		items = append(items, M("vice.feature_spread", "items", sp))
	}
	if len(items) == 0 {
		return Msg{}
	}
	s.Mu.Lock()
	s.gate.structureAsked = true
	s.Mu.Unlock()
	return M("vice.structure", "items", items)
}

func (s *Session) changedCode(changed []CheckpointFile) bool {
	for _, c := range changed {
		if codeExts[strings.ToLower(path.Ext(c.Path))] {
			return true
		}
	}
	return false
}

// deadExports names the dead symbols of the changed files that this turn introduced.
func (s *Session) deadExports(m *ProjectMap, changed []CheckpointFile) string {
	dead := map[string][]string{}
	for _, d := range m.Dead {
		dead[d.File] = append(dead[d.File], d.Symbol)
	}
	var out []string
	for _, c := range changed {
		syms := dead[c.Path]
		if len(syms) == 0 {
			continue
		}
		before := s.checkpointText(c)
		for _, sym := range syms {
			if !strings.Contains(before, sym) {
				out = append(out, c.Path+": "+sym)
			}
		}
	}
	return strings.Join(firstN(out, 12), "; ")
}

// checkpointText is a changed file as it was before the turn ("" for a new file).
func (s *Session) checkpointText(c CheckpointFile) string {
	if c.New {
		return ""
	}
	s.Mu.Lock()
	turn := s.Turn
	s.Mu.Unlock()
	b, err := os.ReadFile(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(turn), filepath.FromSlash(c.Path)))
	if err != nil {
		return ""
	}
	return string(b)
}

// scatteredFiles names new files placed in a folder no import ties to the rest of their feature.
func scatteredFiles(m *ProjectMap, changed []CheckpointFile) string {
	const minimumFolders = 3 // two folders are a feature and its caller
	var out []string
	for _, c := range changed {
		if !c.New {
			continue
		}
		f, ok := projectmap.FeatureOf(m, c.Path)
		if ok && len(f.Dirs) >= minimumFolders && contains(f.Unlinked, path.Dir(c.Path)) {
			out = append(out, fmt.Sprintf("%s (feature %s: %s)", c.Path, f.Name, strings.Join(firstN(f.Dirs, 6), ", ")))
		}
	}
	return strings.Join(firstN(out, 6), "; ")
}

// uiChangeVice tells markup and style changes from other screens' code.
func uiChangeVice(files []string) string {
	for _, f := range files {
		switch strings.ToLower(path.Ext(f)) {
		case ".css", ".scss", ".html":
		default:
			return "screenshot-missing"
		}
	}
	return "surface-change"
}
