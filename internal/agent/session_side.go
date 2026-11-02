package agent

import (
	"context"
	"encoding/json"
	"strings"
)

// Side questions, context breakdown and next-message suggestion.

func (s *Session) SideQuestion(ctx context.Context, question string) (string, error) {
	if stop, over := s.overDailyLimit(); over {
		return "", userError(s.uiLang(), stop)
	}
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return "", err
	}
	msgs := s.copyMessages()
	// a trailing tool_use without its result would be rejected; cut back to the last complete exchange
	for len(msgs) > 0 && msgs[len(msgs)-1].Role == "assistant" {
		hasTool := false
		for _, b := range msgs[len(msgs)-1].Content {
			if b.Type == "tool_use" {
				hasTool = true
			}
		}
		if !hasTool {
			break
		}
		msgs = msgs[:len(msgs)-1]
	}
	if len(msgs) > 0 && msgs[len(msgs)-1].Role == "user" {
		msgs = append(msgs, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "(continuing)"}}})
	}
	msgs = append(msgs, Message{Role: "user", Content: []Block{{Type: "text", Text: "[A side question from the user — answer directly, without tools; this does not enter the history]\n" + question}}})
	res, err := p.Stream(ctx, ModelRequest{Model: s.Model, System: "You answer quick questions about the work session in progress. " + responseLanguageRule(s.settings.Language), Messages: msgs, MaxTokens: 2000}, func(StreamEvent) {})
	if err != nil {
		return "", err
	}
	out := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			out += b.Text
		}
	}
	s.spend(s.Model, res.Usage)
	return out, nil
}

// ContextBreakdown estimates where the context window goes (4 characters ≈ 1 token).
func (s *Session) ContextBreakdown() map[string]any {
	rules := LoadRules(s.Root)
	sys := len(buildSystemPrompt(s, rules)) / 4
	tb, _ := json.Marshal(toolSpecs())
	tools := len(tb) / 4
	var user, assistant, calls, results, images int
	for _, m := range s.copyMessages() {
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if m.Role == "user" {
					user += len(b.Text) / 4
				} else {
					assistant += len(b.Text) / 4
				}
			case "thinking":
				assistant += len(b.Thinking) / 4
			case "tool_use":
				calls += (len(b.Input) + len(b.Name)) / 4
			case "tool_result":
				results += len(b.Content) / 4
			case "image":
				images += 1600
			}
		}
	}
	total := sys + tools + user + assistant + calls + results + images
	limit := s.contextLimit()
	return map[string]any{
		"limit": limit, "total": total,
		"categories": []map[string]any{
			{"name": "System instructions", "tokens": sys}, {"name": "Tools", "tokens": tools},
			{"name": "Your messages", "tokens": user}, {"name": "Responses", "tokens": assistant},
			{"name": "Tool calls", "tokens": calls}, {"name": "Tool results", "tokens": results},
			{"name": "Imagens", "tokens": images},
		},
	}
}

// SuggestNext guesses the user's likely next message from the end of the conversation, for the gray
// suggestion in the prompt box (Tab accepts). The fast model sees only the recent text, no tools.
func (s *Session) SuggestNext(ctx context.Context) (string, error) {
	if _, over := s.overDailyLimit(); over {
		return "", nil
	}
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return "", err
	}
	msgs := s.copyMessages()
	var lines []string
	for i := len(msgs) - 1; i >= 0 && len(lines) < 6; i-- {
		if msgs[i].Kind != "" || msgs[i].Role == "user" && !isUserRequest(msgs[i]) {
			continue // infuse's notes and tool results are not the conversation between the two
		}
		if t := strings.TrimSpace(msgText(msgs[i])); t != "" {
			who := "Agent"
			if msgs[i].Role == "user" {
				who = "User"
			}
			if len(t) > 1200 {
				t = t[:1200] + "…"
			}
			lines = append([]string{who + ": " + t}, lines...)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	ask := "A recent conversation between a user and a coding agent:\n\n" + strings.Join(lines, "\n\n") +
		"\n\nWrite the user's most likely next message, short (up to 12 words), in the user's language. Answer with the message only, without quotes. If there is no clear next step, answer with a dash (-)."
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: ask}}}}, MaxTokens: 60}, func(StreamEvent) {})
	if err != nil {
		return "", err
	}
	s.spend(s.fastModel(), res.Usage)
	out := strings.Trim(strings.TrimSpace(msgText(Message{Content: res.Content})), "\"'“”")
	if out == "-" || out == "" || strings.Contains(out, "\n") || len([]rune(out)) > 120 {
		return "", nil
	}
	for i := len(msgs) - 1; i >= 0 && i >= len(msgs)-12; i-- { // never suggest what the user already sent
		if msgs[i].Role == "user" && strings.EqualFold(strings.TrimSpace(msgText(msgs[i])), out) {
			return "", nil
		}
	}
	return out, nil
}

// msgText joins the text blocks of a message.
