package agent

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// End-of-turn obligations and task closure.

func (s *Session) turnObligation(ctx context.Context) Msg {
	changed, open, asks := s.turnState()
	if asks >= 6 { // the agent keeps refusing: the turn ends, unverified — never as done
		s.closeOpenTasks(M("plan.closed_note").Render(s.uiLang())) // shown only in the plan pane
		s.markUnverified()
		return Msg{}
	}
	s.Mu.Lock()
	snapErr := s.snapshotErr
	s.Mu.Unlock()
	if snapErr != nil { // without the initial tree there is nothing to compare against: fail closed
		return M("turn.snapshot", "error", snapErr.Error())
	}
	if len(open) > 0 {
		return M("turn.open_tasks", "tasks", strings.Join(open, "; "))
	}
	// whatever changed the tree outside the write tools is judged before anything else
	if why := s.auditShellWrites(); !why.Empty() {
		return why
	}
	// a turn that only started a server changed no file and still owes an account of it
	if why := s.missingDeclaredProcs(); !why.Empty() {
		return why
	}
	if len(changed) == 0 {
		// A turn that changed nothing still owes the oracle its verdict: the content-hash cache reuses
		// the approved run, so the check is cheap, and a criterion can never quietly stop governing.
		if why := s.oracleObligation(ctx); !why.Empty() {
			return why
		}
		return Msg{}
	}
	// the project's checker on every file changed in the turn
	if why := s.checkerObligation(changed); !why.Empty() {
		return why
	}
	if why := s.structureObligation(changed); !why.Empty() { // dead exports and a scattered feature, once
		return why
	}
	if why := s.missingVerification(); !why.Empty() {
		return why
	}
	if why := s.missingReport(); !why.Empty() {
		return why
	}
	if why := s.unconfirmedReport(); !why.Empty() {
		return why
	}
	if why := s.oracleObligation(ctx); !why.Empty() { // the verdict, run by infuse on the current tree
		return why
	}
	if why := s.reviewObligation(ctx); !why.Empty() {
		return why
	}
	s.noteUnmeasuredContract()
	return Msg{}
}

// reviewObligation runs the independent review once per version of the work: findings send the agent
// back and the answer is reviewed again. A review that could not run is never an approval.
func (s *Session) reviewObligation(ctx context.Context) Msg {
	if s.settings.RequireReview != nil && !*s.settings.RequireReview {
		return Msg{}
	}
	s.Mu.Lock()
	reviewed := s.gate.reviewed
	s.gate.reviewed = true
	s.Mu.Unlock()
	if reviewed {
		return Msg{}
	}
	out, err := s.RunSubagent(ctx, "review", M("turn.review_title").Render(s.uiLang()), s.reviewMaterial("")+"\nReview the changes above. End with a line exactly like this: VERDICT: APPROVE or VERDICT: FIX.")
	if err != nil {
		s.setOutcome("unverified", M("turn.review_failed", "error", firstLine(err.Error())))
		return Msg{}
	}
	if regexp.MustCompile(`(?i)(VERDICT:\s*APPROVE|VEREDITO:\s*APROVAR)`).MatchString(out) {
		return Msg{}
	}
	s.Mu.Lock()
	s.gate.reviewed = false
	s.Mu.Unlock()
	return M("turn.review", "findings", out)
}

// noteUnmeasuredContract tells the user when confirmed criteria have no executable check: the contract
// then lives only in the agent's memory and the executor measures nothing.
func (s *Session) noteUnmeasuredContract() {
	s.Mu.Lock()
	c := s.Contract
	turn := s.Turn
	s.Mu.Unlock()
	if c == nil || c.Turn != turn || exists(filepath.Join(s.Root, infuseDirName, "oracle", "checks.json")) {
		return
	}
	s.emitMsg("gate", M("contract.unmeasured"))
}

// checkerObligation runs the project's checker on every file the turn changed.
func (s *Session) checkerObligation(changed []CheckpointFile) Msg {
	var problems []string
	for _, c := range changed {
		abs := filepath.Join(s.workRoot(), filepath.FromSlash(c.Path))
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if d := fileDiagnostics(s.Root, abs); d != "" {
			problems = append(problems, c.Path+":\n"+d)
		}
	}
	if len(problems) > 0 {
		return M("turn.checker", "problems", truncate(strings.Join(problems, "\n\n"), 4000))
	}
	return Msg{}
}

// markUnverified records a turn that ended with obligations still open: the user sees it, the journal
// keeps it, and the turn is never counted as done.
func (s *Session) markUnverified() {
	s.setOutcome("unverified", M("turn.unverified"))
}

// setOutcome records how a turn really ended: journaled, kept on the session, shown to the user.
func (s *Session) setOutcome(outcome string, why Msg) {
	Journal(s.Root, JournalEntry{Kind: "gate", Detail: "turn ended " + outcome + ": " + why.Render("en")})
	s.Mu.Lock()
	s.Outcome = outcome
	s.Mu.Unlock()
	s.emitMsg("gate", why)
}

// turnState is what the obligations need: the files the turn changed (by any tool), the plan's open
// tasks and how many times the agent was already sent back.
func (s *Session) turnState() (changed []CheckpointFile, open []string, asks int) {
	s.Mu.Lock()
	changed = append([]CheckpointFile{}, s.Checkpoints[s.Turn]...)
	asks = s.gate.obligationAsks
	for _, t := range s.Tasks {
		if t.Status != "done" && t.Status != "blocked" {
			open = append(open, t.Text)
		}
	}
	s.Mu.Unlock()
	for _, ch := range s.ungatedChanges() {
		if !ch.Deleted {
			changed = append(changed, CheckpointFile{Path: ch.Path, New: ch.New})
		}
	}
	return changed, open, asks
}

// missingDeclaredProcs: a turn never ends leaving a background process it did not account for. Every
// running process must be named in the turn's Report (its id and what it serves) or stopped.
func (s *Session) missingDeclaredProcs() Msg {
	s.Mu.Lock()
	start := 0
	if s.Turn >= 1 && s.Turn-1 < len(s.TurnStarts) {
		start = s.TurnStarts[s.Turn-1]
	}
	var report strings.Builder
	for _, m := range s.Messages[min(start, len(s.Messages)):] {
		for _, b := range m.Content {
			if b.Type == "tool_use" && b.Name == "Report" {
				report.Write(b.Input)
				report.WriteString("\n")
			}
		}
	}
	var open []string
	for _, p := range s.procs {
		if p.State != "running" {
			continue
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(p.ID) + `\b`).MatchString(report.String()) {
			open = append(open, p.ID+" ("+truncate(p.Command, 80)+")")
		}
	}
	s.Mu.Unlock()
	if len(open) == 0 {
		return Msg{}
	}
	return M("turn.open_procs", "items", strings.Join(open, "; "))
}

// closeOpenTasks marks every open task blocked with infuse's note: the plan never shows work as
// ongoing once the agent has stopped.
func (s *Session) closeOpenTasks(note string) {
	s.Mu.Lock()
	tasks := append([]TaskItem{}, s.Tasks...)
	s.Mu.Unlock()
	changed := false
	for i := range tasks {
		if tasks[i].Status != "done" && tasks[i].Status != "blocked" {
			tasks[i].Status, tasks[i].Note, changed = "blocked", note, true
		}
	}
	if changed {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "plan closed with open tasks"})
		s.setTasks(tasks)
	}
}

// hostOf returns a URL's host (for messages).
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return raw
}
