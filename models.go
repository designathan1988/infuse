package main

// What each model accepts, from the providers' official model tables (read 2026-10-31):
//   Claude: https://platform.claude.com/docs/en/models/overview — every Claude 5 model: 1M context, 128K output.
//   DeepSeek: https://api-docs.deepseek.com/quick_start/pricing — deepseek-flash and deepseek-v4-pro: 1M
//   context, 384K output; the legacy names are served by the same models.
// The context window decides when a conversation is compacted; it is never a fixed number.

import "strings"

type modelLimit struct{ context, output int }

var modelLimits = map[string]modelLimit{
	"claude-fable-5-1":  {1_000_000, 128_000},
	"claude-opus-5-5":   {1_000_000, 128_000},
	"claude-sonnet-5-5": {1_000_000, 128_000},
	"claude-haiku-5-5":  {1_000_000, 128_000},
	"deepseek-flash":    {1_000_000, 384_000},
	"deepseek-v4-flash": {1_000_000, 384_000},
	"deepseek-v4-pro":   {1_000_000, 384_000},
	"deepseek-chat":     {1_000_000, 384_000},
	"deepseek-reasoner": {1_000_000, 384_000},
}

// fallbackContext is used for a model infuse has no official figure for (a local or unknown model).
const fallbackContext = 128_000

// contextWindow is the model's context in tokens: its official figure, or its family's.
func contextWindow(model string) int {
	m := strings.ToLower(strings.TrimSpace(model))
	if l, ok := modelLimits[m]; ok {
		return l.context
	}
	for _, fam := range []string{"claude-fable-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5", "deepseek-"} {
		if strings.HasPrefix(m, fam) {
			for id, l := range modelLimits {
				if strings.HasPrefix(id, fam) {
					return l.context
				}
			}
		}
	}
	return fallbackContext
}

// contextLimit is the session's working context: the model's window, or less when the user set less.
func (s *Session) contextLimit() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contextLimitLocked()
}

func (s *Session) contextLimitLocked() int {
	model := s.Model
	if model == "" {
		model = s.settings.Model
	}
	window := contextWindow(model)
	if lim := s.settings.ContextLimit; lim > 0 && lim < window {
		return lim
	}
	return window
}

// repairTranscript makes the conversation valid for any provider: a tool result must answer a call in
// the assistant message right before it, and every call must have its result. Whatever breaks that (an
// interrupted turn, an old compaction) is turned into plain text instead of reaching the provider and
// failing the session.
func repairTranscript(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for i, m := range msgs {
		m.Content = append([]Block{}, m.Content...)
		if m.Role == "user" {
			calls := map[string]bool{}
			if len(out) > 0 && out[len(out)-1].Role == "assistant" {
				for _, b := range out[len(out)-1].Content {
					if b.Type == "tool_use" {
						calls[b.ID] = true
					}
				}
			}
			for j, b := range m.Content {
				if b.Type == "tool_result" && !calls[b.ToolUseID] {
					m.Content[j] = Block{Type: "text", Text: "[resultado de ferramenta sem a chamada correspondente]\n" + truncate(b.Content, 4000)}
				}
			}
		}
		out = append(out, m)
		if m.Role != "assistant" {
			continue
		}
		// every call of this message needs its result in the next message
		answered := map[string]bool{}
		if i+1 < len(msgs) && msgs[i+1].Role == "user" {
			for _, b := range msgs[i+1].Content {
				if b.Type == "tool_result" {
					answered[b.ToolUseID] = true
				}
			}
		}
		var missing []Block
		for _, b := range m.Content {
			if b.Type == "tool_use" && !answered[b.ID] {
				missing = append(missing, Block{Type: "tool_result", ToolUseID: b.ID, Content: "sem resultado: a execução foi interrompida", IsError: true})
			}
		}
		if len(missing) > 0 {
			if i+1 < len(msgs) && msgs[i+1].Role == "user" {
				msgs[i+1].Content = append(append([]Block{}, missing...), msgs[i+1].Content...)
			} else {
				out = append(out, Message{Role: "user", Content: missing})
			}
		}
	}
	return out
}
