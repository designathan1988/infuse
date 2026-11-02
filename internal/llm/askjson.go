package llm

// The model as a compile step. infuse asks the model to turn text (instructions, the user's messages, a
// document) into structured data, then checks every item mechanically. The answer must arrive whole: a
// malformed answer read as "nothing found" would silently switch a lock off. So the answer is a forced
// call to one tool whose parameters are the schema — each provider's own way to make the model answer in
// that shape (Anthropic tool_choice {"type":"tool"} where the model accepts it, else a strict tool under
// tool_choice auto and the order in the prompt — anthropic_request.go —, OpenAI-compatible and DeepSeek tool_choice with a
// named function — DeepSeek only with thinking disabled, https://api-docs.deepseek.com/api/create-chat-completion
// read 2026-10-31 —, Gemini functionCallingConfig ANY, Ollama's format schema). An answer that still does
// not parse goes back to the model with the parser's error, and after the last attempt the caller gets
// an error to show, never an empty result.

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"strings"
)

const AskJSONAttempts = 3

// AskJSON asks model for the arguments of the tool spec and decodes them into out.
func AskJSON(ctx context.Context, p Provider, model, system, user string, spec ToolSpec, maxTokens int, out any) (Usage, error) {
	msgs := []Message{{Role: "user", Content: []Block{{Type: "text", Text: user}}}}
	var total Usage
	var last error
	for attempt := 0; attempt < AskJSONAttempts; attempt++ {
		res, err := p.Stream(ctx, ModelRequest{Model: model, System: system, Messages: msgs, Tools: []ToolSpec{spec}, ForceTool: spec.Name, MaxTokens: maxTokens}, func(StreamEvent) {})
		if err != nil {
			return total, err
		}
		total = addUsage(total, res.Usage)
		raw, callID := answerJSON(res.Content, spec.Name)
		if last = decodeStrict(raw, out); last == nil {
			return total, nil
		}
		if res.StopReason == "max_tokens" || res.StopReason == "length" {
			last = fmt.Errorf("the answer was cut at the output limit (%d tokens): %w", maxTokens, last)
		}
		msgs = append(msgs, Message{Role: "assistant", Content: res.Content})
		fix := fmt.Sprintf("Those arguments are not valid JSON for %s: %v. Call %s again with the complete, valid arguments; keep only what fits, nothing invented.", spec.Name, last, spec.Name)
		if callID != "" {
			msgs = append(msgs, Message{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: callID, Content: fix, IsError: true}}})
		} else {
			msgs = append(msgs, Message{Role: "user", Content: []Block{{Type: "text", Text: fix}}})
		}
	}
	return total, fmt.Errorf("no valid answer after %d attempts: %w", AskJSONAttempts, last)
}

// answerJSON is the JSON the model gave: the forced call's arguments, or a JSON object in its text
// (providers without a forced call).
func answerJSON(content []Block, tool string) (raw, callID string) {
	for _, b := range content {
		if b.Type == "tool_use" && b.Name == tool {
			return string(b.Input), b.ID
		}
	}
	text := core.TextOf(Message{Content: content})
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		return text[i : j+1], ""
	}
	return text, ""
}

// decodeStrict decodes raw into out and refuses an empty answer.
func decodeStrict(raw string, out any) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("empty answer")
	}
	return json.Unmarshal([]byte(raw), out)
}

func addUsage(a, b Usage) Usage {
	a.Input += b.Input
	a.Output += b.Output
	a.CacheRead += b.CacheRead
	a.CacheWrite += b.CacheWrite
	return a
}
