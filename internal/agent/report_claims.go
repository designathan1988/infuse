package agent

// Claim verification: every claim of a Report is compared with the turn's tool calls, its checkpoints
// and the lines infuse actually showed the agent. What a subagent did is not the agent's own evidence
// (F6): it is reported as partial until the agent itself produces the evidence.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// verifyClaim compares one claim with the turn's tool calls and checkpoints.
func (s *Session) verifyClaim(c *ReportClaim) {
	ctx := s.claimContext()
	switch c.Kind {
	case "command":
		s.verifyCommandClaim(c, ctx)
	case "file":
		verifyFileClaim(c, ctx)
	case "read": // "I read X": the lines infuse showed the agent, not its word
		s.verifyReadClaim(c, ctx)
	case "url": // "this page says…": the page was fetched in this session
		if len(s.unfetchedCitations(c.Ref)) == 0 && strings.TrimSpace(c.Ref) != "" {
			c.Status, c.Evidence = "confirmado", M("report.ev_fetched")
		} else {
			c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_fetched")
		}
	default:
		c.Kind, c.Status, c.Evidence = "other", "sem-evidencia", M("report.ev_none")
	}
}

func (s *Session) verifyCommandClaim(c *ReportClaim, ctx claimContext) {
	ref := squash(c.Ref)
	if ref == "" {
		c.Status, c.Evidence = "sem-evidencia", M("report.ev_no_command")
		return
	}
	results := map[string]Block{}
	for _, m := range ctx.msgs {
		for _, b := range m.Content {
			if b.Type == "tool_result" {
				results[b.ToolUseID] = b
			}
		}
	}
	found, failed := false, ""
	for _, m := range ctx.msgs {
		for _, b := range m.Content {
			if b.Type != "tool_use" || b.Name != "Bash" {
				continue
			}
			var in struct {
				Command    string
				Background bool `json:"run_in_background"`
			}
			_ = json.Unmarshal(b.Input, &in)
			if squash(in.Command) != ref { // B2: the command the claim names, exactly (spacing aside)
				continue
			}
			// A background command's first seconds are not its result: only the final exit code counts.
			if in.Background {
				if id, state, code, ok := s.backgroundOutcome(in.Command); ok {
					switch {
					case state == "running":
						c.Status, c.Evidence = "nao-confirmado", M("report.ev_bg_running", "id", id)
					case code != 0:
						c.Status, c.Evidence = "nao-confirmado", M("report.ev_bg_failed", "id", id, "exit", code)
					default:
						c.Status, c.Evidence = "confirmado", M("report.ev_bg_done", "id", id)
					}
					return
				}
			}
			r, ok := results[b.ID]
			if ok && !r.IsError {
				found = true
			} else if ok {
				failed = firstLine(r.Content)
			}
		}
	}
	switch {
	case found:
		c.Status, c.Evidence = "confirmado", M("report.ev_ran")
	case failed != "":
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_failed", "output", failed)
	case s.subagentRan(ref): // F6: a subagent's run is not the agent's own execution
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_subagent_ran")
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_run")
	}
}

// backgroundOutcome finds the background process a command started and returns its final state.
func (s *Session) backgroundOutcome(cmd string) (id, state string, code int, ok bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	for i := len(s.procs) - 1; i >= 0; i-- {
		p := s.procs[i]
		if squash(p.Original) == squash(cmd) || squash(p.Command) == squash(cmd) {
			if p.Exit != nil {
				code = *p.Exit
			}
			return p.ID, p.State, code, true
		}
	}
	return "", "", 0, false
}

// mainWrote reports whether the agent itself wrote rel in this turn.
func mainWrote(ctx claimContext, rel string) bool {
	for _, m := range ctx.msgs {
		for _, b := range m.Content {
			if b.Type != "tool_use" || (b.Name != "Write" && b.Name != "Edit" && b.Name != "MultiEdit") {
				continue
			}
			var in struct{ Path, FilePath string }
			_ = json.Unmarshal(b.Input, &in)
			if claimPath(ctx, firstNonEmpty(in.Path, in.FilePath)) == rel {
				return true
			}
		}
	}
	return false
}

// mainRead reports whether the agent itself read rel in this turn.
func mainRead(ctx claimContext, rel string) bool {
	for _, m := range ctx.msgs {
		for _, b := range m.Content {
			if b.Type != "tool_use" || b.Name != "Read" {
				continue
			}
			var in struct{ Path, FilePath string }
			_ = json.Unmarshal(b.Input, &in)
			if claimPath(ctx, firstNonEmpty(in.Path, in.FilePath)) == rel {
				return true
			}
		}
	}
	return false
}

// claimPath is a tool path compared the way the claim compares: relative to the work root, slash
// separators, single spaces.
func claimPath(ctx claimContext, p string) string {
	if filepath.IsAbs(p) {
		if rel, err := filepath.Rel(ctx.root, p); err == nil {
			p = rel
		}
	}
	return squash(filepath.ToSlash(strings.TrimPrefix(p, "./")))
}

// subagentRan: a subagent of this session ran the command in its own trail.
func (s *Session) subagentRan(cmd string) bool {
	s.Mu.Lock()
	runs := append([]AgentRun{}, s.Agents...)
	s.Mu.Unlock()
	for _, a := range runs {
		for _, st := range a.Trail {
			if st.Kind != "tool" || st.Tool != "Bash" {
				continue
			}
			var in struct{ Command string }
			if json.Unmarshal([]byte(st.Input), &in) == nil && squash(in.Command) == cmd {
				return true
			}
		}
	}
	return false
}

func verifyFileClaim(c *ReportClaim, ctx claimContext) {
	rel := claimPath(ctx, c.Ref)
	_, err := os.Stat(filepath.Join(ctx.root, filepath.FromSlash(rel)))
	switch {
	case ctx.written[rel] && err == nil && mainWrote(ctx, rel):
		c.Status, c.Evidence = "confirmado", M("report.ev_changed")
	case ctx.written[rel] && err == nil: // F6: written by a subagent, not by the agent itself
		c.Status, c.Evidence = "parcial", M("report.ev_subagent")
	case ctx.written[rel]:
		c.Status, c.Evidence = "confirmado", M("report.ev_removed")
	case err == nil:
		c.Status, c.Evidence = "parcial", M("report.ev_unchanged")
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_missing")
	}
}

func (s *Session) verifyReadClaim(c *ReportClaim, ctx claimContext) {
	rel := claimPath(ctx, c.Ref)
	abs := filepath.Join(ctx.root, filepath.FromSlash(rel))
	seen, total, _ := s.readProgress(abs)
	switch {
	case total > 0 && seen >= total && mainRead(ctx, rel):
		c.Status, c.Evidence = "confirmado", M("report.ev_read_full", "total", total)
	case total > 0 && seen >= total: // F6: a subagent read it, not the agent
		c.Status, c.Evidence = "parcial", M("report.ev_subagent")
	case seen > 0:
		c.Status, c.Evidence = "parcial", M("report.ev_read_partial", "seen", seen, "total", total)
	default:
		c.Status, c.Evidence = "nao-confirmado", M("report.ev_not_read")
	}
}

// claimStatus is a claim's status code as a message.
func claimStatus(code string) Msg {
	switch code {
	case "confirmado":
		return M("report.st_confirmed")
	case "parcial":
		return M("report.st_partial")
	case "nao-confirmado":
		return M("report.st_unconfirmed")
	}
	return M("report.st_no_evidence")
}
