package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// Context measurement and compaction.

func (s *Session) needsCompaction() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	// Estimate: 4 chars per token over the serialized messages.
	b, _ := json.Marshal(s.Messages)
	return len(b)/4 > int(float64(s.contextLimitLocked())*0.75)
}

func (s *Session) compact(ctx context.Context, p Provider) error {
	s.setState("compacting")
	msgs := s.copyMessages()
	if len(msgs) < 4 {
		return nil
	}
	// the latest exchange stays verbatim; both are fresh slices: appending the request below must not
	// write over the exchange that is kept (a shared backing array lost the agent's tool calls once)
	keepFrom := len(msgs) - 2
	keep := append([]Message{}, msgs[keepFrom:]...)
	older := append([]Message{}, msgs[:keepFrom]...)
	req, reqOK := s.currentRequestIn(msgs)
	older = append(older, Message{Role: "user", Content: []Block{{Type: "text", Text: compactRequest + " Be specific and concise."}}})
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), Messages: repairTranscript(older), MaxTokens: 4000}, func(StreamEvent) {})
	if err != nil {
		return err
	}
	summary := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			summary += b.Text
		}
	}
	s.Mu.Lock()
	head := compactionHead("[Context compacted — summary of the work so far]\n\n"+summary, verbatimRequests(older[:len(older)-1]))
	s.compactedTranscriptLocked(head, keep, keepFrom, req, reqOK)
	s.Compactions++
	s.Cost += s.settings.CostOf(s.fastModel(), res.Usage)
	s.Mu.Unlock()
	s.emit("compacted", map[string]any{"summary": summary})
	return nil
}

// currentRequestIn is the message that opened the current turn, in msgs.
func (s *Session) currentRequestIn(msgs []Message) (Message, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.Turn >= 1 && s.Turn-1 < len(s.TurnStarts) {
		if i := s.TurnStarts[s.Turn-1]; i >= 0 && i < len(msgs) {
			return msgs[i], true
		}
	}
	return Message{}, false
}

// compactedTranscriptLocked replaces the conversation and rebuilds the turn index table. The current
// turn keeps pointing at its own request: when the request fell in the summarized part, it is put back
// verbatim in front of the kept exchange. Earlier turns point at the head: their messages are a summary.
func (s *Session) compactedTranscriptLocked(head, keep []Message, keepFrom int, req Message, reqOK bool) {
	msgs := append([]Message{}, head...)
	pos := -1
	if s.Turn >= 1 && s.Turn-1 < len(s.TurnStarts) {
		switch old := s.TurnStarts[s.Turn-1]; {
		case old >= keepFrom:
			pos = len(head) + old - keepFrom
		case reqOK:
			msgs = append(msgs, req)
			pos = len(head)
		}
	}
	msgs = append(msgs, keep...)
	s.Messages = msgs
	starts := make([]int, max(s.Turn, 1))
	if pos >= 0 {
		starts[len(starts)-1] = pos
	}
	s.TurnStarts = starts
}

// compactRequest is what the model is asked when the conversation is summarized.
const compactRequest = "Summarize the conversation above so the work can continue without losing anything essential: the goal, the decisions made, the files created or changed (with paths), what was tried and rejected, the errors found, the current state and the next steps."

// compactionHead is the start of a compacted conversation: the summary, then the user's own requests,
// each verbatim in its own block (the next compaction takes them back from there), then the agent's
// acknowledgement. Both messages are marked as the compaction, never read from their text.
func compactionHead(summary string, requests []string) []Message {
	blocks := []Block{{Type: "text", Text: summary}}
	if len(requests) > 0 {
		blocks = append(blocks, Block{Type: "text", Text: "## The user's requests, verbatim (oldest first; each block below is one request, word for word)"})
		for _, r := range requests {
			blocks = append(blocks, Block{Type: "text", Text: r})
		}
	}
	return []Message{
		{Role: "user", Kind: "compaction", Content: blocks, TS: time.Now().UnixMilli()},
		{Role: "assistant", Kind: "compaction", Content: []Block{{Type: "text", Text: "Understood; I continue from that state."}}},
	}
}

// verbatimRequests returns what the user typed in msgs, word for word, newest kept whole when the
// budget runs out: a summary may lose a nuance; these islands keep the user's own wording.
func verbatimRequests(msgs []Message) []string {
	var reqs []string
	for _, m := range msgs {
		if m.Role == "user" && m.Kind == "compaction" { // islands kept by an earlier compaction
			for i, b := range m.Content {
				if i >= 2 && b.Type == "text" {
					reqs = append(reqs, b.Text)
				}
			}
			continue
		}
		if !isUserRequest(m) {
			continue
		}
		for _, b := range m.Content {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				reqs = append(reqs, strings.TrimSpace(b.Text))
				break
			}
		}
	}
	const budget = 6000
	var out []string
	used := 0
	for i := len(reqs) - 1; i >= 0; i-- {
		r := reqs[i]
		if used+len(r) > budget {
			if used > 0 {
				break // the oldest requests do not fit: the summary covers them
			}
			r = r[:budget] + " […]"
		}
		used += len(r)
		out = append(out, r)
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// ---------------------------------------------------------------- rewind variants, summaries, side chat

// RewindTo restores files and/or the conversation to before turn toTurn. It returns the prompt that
// started that turn, so the window can put it back in the prompt box.
