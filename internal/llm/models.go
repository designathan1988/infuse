package llm

// What each model accepts, from the providers' official model tables (read 2026-10-31):
//   Claude: https://platform.claude.com/docs/en/models/overview — every Claude 5 model: 1M context, 128K output.
//   DeepSeek: https://api-docs.deepseek.com/quick_start/pricing — deepseek-flash and deepseek-v4-pro: 1M
//   context, 384K output; the legacy names are served by the same models.
// The context window decides when a conversation is compacted; it is never a fixed number.

import (
	"infuse/internal/core"
	"sort"
	"strings"
)

type modelLimit struct{ context, output int }

var modelLimits = map[string]modelLimit{
	"claude-fable-5-1":  {1_000_000, 128_000},
	"claude-fable-5":    {1_000_000, 128_000},
	"claude-opus-5-5":   {1_000_000, 128_000},
	"claude-opus-5":     {1_000_000, 128_000},
	"claude-opus-4-8":   {1_000_000, 128_000},
	"claude-opus-4-7":   {1_000_000, 128_000},
	"claude-opus-4-6":   {1_000_000, 128_000},
	"claude-sonnet-5-5": {1_000_000, 128_000},
	"claude-sonnet-5":   {1_000_000, 128_000},
	"claude-sonnet-4-6": {1_000_000, 128_000},
	"claude-haiku-5-5":  {1_000_000, 128_000},
	"claude-haiku-4-5":  {200_000, 64_000},
	"deepseek-flash":    {1_000_000, 384_000},
	"deepseek-v4-flash": {1_000_000, 384_000},
	"deepseek-v4-pro":   {1_000_000, 384_000},
	"deepseek-chat":     {1_000_000, 384_000},
	"deepseek-reasoner": {1_000_000, 384_000},
}

// FallbackContext is used for a model infuse has no official figure for (a local or unknown model).
const FallbackContext = 128_000

// ContextWindow is the model's context in tokens: its official figure, or its family's.
func ContextWindow(model string) int {
	m := bareModel(model)
	if l, ok := modelLimits[m]; ok {
		return l.context
	}
	for _, fam := range []string{"claude-haiku-4-5", "claude-fable-5", "claude-opus-5", "claude-sonnet-5", "claude-haiku-5", "deepseek-"} {
		if strings.HasPrefix(m, fam) {
			for id, l := range modelLimits {
				if strings.HasPrefix(id, fam) {
					return l.context
				}
			}
		}
	}
	return FallbackContext
}

// OutputLimit is how many tokens a reply may use: the model's official maximum, up to 64K (a reply
// cut short is recovered anyway, but should be rare).
func OutputLimit(model string, configured int) int {
	if configured > 0 && configured != 16000 { // 16000 was a fixed default, never a choice
		return configured
	}
	l, ok := modelLimits[bareModel(model)]
	if !ok {
		return 16000
	}
	return min(l.output, 64000)
}

// bareModel is the id without what a cloud wraps around it (anthropic.claude-…, us.anthropic.claude-…).
func bareModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.Index(m, "claude-"); i > 0 {
		return m[i:]
	}
	return m
}

// ProviderNames lists the registered provider kinds.
func ProviderNames() []string {
	out := make([]string, 0, len(providers))
	for k := range providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// RepairTranscript makes the conversation valid for any provider: a tool result must answer a call in
// the assistant message right before it, and every call must have its result. Whatever breaks that (an
// interrupted turn, an old compaction) is turned into plain text instead of reaching the provider and
// failing the session.
func RepairTranscript(msgs []Message) []Message {
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
					m.Content[j] = Block{Type: "text", Text: "[resultado de ferramenta sem a chamada correspondente]\n" + core.Truncate(b.Content, 4000)}
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
				missing = append(missing, Block{Type: "tool_result", ToolUseID: b.ID, Content: "no result: the execution was interrupted", IsError: true})
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
