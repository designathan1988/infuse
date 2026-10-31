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
	s.Mu.Lock()
	changed := append([]CheckpointFile{}, s.Checkpoints[s.Turn]...)
	asks := s.gate.obligationAsks
	var open []string
	for _, t := range s.Tasks { // the session's plan, whichever turn registered it
		if t.Status != "done" && t.Status != "blocked" {
			open = append(open, t.Text)
		}
	}
	s.Mu.Unlock()
	if asks >= 6 { // the agent keeps refusing: the turn ends, and infuse closes what was left as blocked
		s.closeOpenTasks(M("plan.closed_note").Render(s.uiLang())) // shown only in the plan pane
		return Msg{}
	}
	if len(open) > 0 {
		return M("turn.open_tasks", "tasks", strings.Join(open, "; "))
	}
	if len(changed) == 0 {
		return Msg{}
	}
	// the project's checker on every file changed in the turn
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
	if why := s.missingVerification(); !why.Empty() {
		return why
	}
	if why := s.missingReport(); !why.Empty() {
		return why
	}
	// the mandatory independent review, once per turn (its findings must be answered)
	if s.settings.RequireReview == nil || *s.settings.RequireReview {
		s.Mu.Lock()
		reviewed := s.gate.reviewed
		s.gate.reviewed = true
		s.Mu.Unlock()
		if !reviewed {
			out, err := s.RunSubagent(ctx, "review", M("turn.review_title").Render(s.uiLang()), s.reviewMaterial("")+"\nReview the changes above. End with a line exactly like this: VERDICT: APPROVE or VERDICT: FIX.")
			if err == nil && !regexp.MustCompile(`(?i)(VERDICT:\s*APPROVE|VEREDITO:\s*APROVAR)`).MatchString(out) {
				return M("turn.review", "findings", out)
			}
		}
	}
	return Msg{}
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
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "plano encerrado com tarefas abertas"})
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
