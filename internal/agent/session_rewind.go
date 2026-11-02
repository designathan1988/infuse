package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Rewind and summarization of earlier turns.

func (s *Session) RewindTo(toTurn int, code, conversation bool) (string, error) {
	s.Mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.Mu.Unlock()
		return "", errors.New(M("session.stop_first_rewind").Render(s.uiLang()))
	}
	if toTurn < 1 || toTurn > s.Turn {
		s.Mu.Unlock()
		return "", errors.New(M("session.no_turn").Render(s.uiLang()))
	}
	prompt := s.rewindPromptLocked(toTurn)
	s.Mu.Unlock()
	if code && conversation {
		return prompt, s.Revert(toTurn)
	}
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if code {
		s.restoreCodeLocked(toTurn)
	}
	if conversation {
		s.cutConversationLocked(toTurn)
	}
	s.saveLocked()
	s.scheduleMapRefreshLocked()
	return prompt, nil
}

func (s *Session) rewindPromptLocked(toTurn int) string {
	if toTurn-1 >= len(s.TurnStarts) {
		return ""
	}
	i := s.TurnStarts[toTurn-1]
	if i >= len(s.Messages) {
		return ""
	}
	for _, b := range s.Messages[i].Content {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

func (s *Session) restoreCodeLocked(toTurn int) {
	for t := s.Turn; t >= toTurn; t-- {
		s.restoreTurnLocked(t)
	}
	Journal(s.Root, JournalEntry{Kind: "revert", Detail: fmt.Sprintf("session %s: code restored to before turn %d", s.ID, toTurn)})
}

func (s *Session) cutConversationLocked(toTurn int) {
	for t := s.Turn; t >= toTurn; t-- { // later checkpoints no longer belong to any turn
		_ = os.RemoveAll(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t)))
		delete(s.Checkpoints, t)
	}
	if toTurn-1 < len(s.TurnStarts) {
		s.Messages = s.Messages[:s.TurnStarts[toTurn-1]]
		s.TurnStarts = s.TurnStarts[:toTurn-1]
	}
	s.Turn = toTurn - 1
}

// SummarizeUpTo compresses the conversation before turn upTo into a summary; later turns stay verbatim.
func (s *Session) SummarizeUpTo(ctx context.Context, upTo int, hint string) error {
	s.Mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.Mu.Unlock()
		return errors.New(M("session.stop_first_summary").Render(s.uiLang()))
	}
	if upTo < 2 || upTo-1 >= len(s.TurnStarts) {
		s.Mu.Unlock()
		return errors.New(M("session.nothing_to_summarize").Render(s.uiLang()))
	}
	cut := s.TurnStarts[upTo-1]
	older := append([]Message{}, s.Messages[:cut]...)
	s.Mu.Unlock()
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return err
	}
	ask := compactRequest
	if strings.TrimSpace(hint) != "" {
		ask += "\nFocus the user asked for: " + hint
	}
	older = append(older, Message{Role: "user", Content: []Block{{Type: "text", Text: ask}}})
	model := s.fastModel()
	res, err := p.Stream(ctx, ModelRequest{Model: model, Messages: older, MaxTokens: 4000}, func(StreamEvent) {})
	if err != nil {
		return err
	}
	s.spend(model, res.Usage)
	summary := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			summary += b.Text
		}
	}
	s.Mu.Lock()
	head := compactionHead("[Conversation summarized]\n\n"+summary, nil)
	rest := s.Messages[cut:]
	shift := cut - len(head)
	starts := compactedStarts(upTo-1, len(head)) // the turn numbers stay: checkpoints are filed under them
	for _, st := range s.TurnStarts[upTo-1:] {
		starts = append(starts, st-shift)
	}
	s.Messages = append(head, rest...)
	s.TurnStarts = starts
	s.Compactions++
	s.Mu.Unlock()
	s.save()
	s.emit("session", s.snapshot())
	return nil
}

// SideQuestion answers a question about the session without adding anything to its history.
