package llm

// The Messages API request: the body for a model family and the conversation replay.

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	bindingBeta = "thinking-binding-controls-2026-08-01"
	// thinkingFloor keeps room for the answer when reasoning cannot be turned off: a small max_tokens
	// (a one-line suggestion) would otherwise be spent on the reasoning alone.
	thinkingFloor = 2048
	// minThinkingBudget is the smallest budget_tokens the API accepts.
	minThinkingBudget = 1024
)

// anthropicBody is the request body and the betas it needs.
func anthropicBody(req ModelRequest) (map[string]any, []string) {
	f := familyOf(req.Model)
	thinking, msgs, betas := f.thinkingOf(req)
	maxTokens := req.MaxTokens
	if f.alwaysOn && req.Thinking <= 0 {
		maxTokens = max(maxTokens, thinkingFloor)
	}
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": maxTokens,
		"messages":   anthropicMessages(msgs),
		"stream":     true,
	}
	if thinking != nil {
		body["thinking"] = thinking
	}
	if e := f.effortOf(req); e != "" {
		body["output_config"] = map[string]any{"effort": e}
	}
	system := req.System
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.ForceTool != "" {
		if f.forceTool {
			body["tool_choice"] = map[string]any{"type": "tool", "name": req.ForceTool}
		} else { // the model rejects a forced call: strict arguments, and the order in the prompt
			body["tool_choice"] = map[string]any{"type": "auto", "disable_parallel_tool_use": true}
			body["tools"] = strictTools(req.Tools)
			system = strings.TrimSpace(system + "\n\n" + fmt.Sprintf("Answer by calling the tool %s exactly once, with the complete arguments. Do not answer in text.", req.ForceTool))
		}
	}
	if system != "" {
		body["system"] = []map[string]any{{"type": "text", "text": system, "cache_control": map[string]string{"type": "ephemeral"}}}
	}
	return body, betas
}

// thinkingOf decides the thinking field and the history to replay. Turning reasoning off drops the
// blocks of the whole history (the API's documented recovery); a request that keeps them on a model
// with the history-editing check asks for "drop_block": infuse's system prompt changes between
// requests, and a block whose prefix changed must be dropped, not fail the turn.
func (f claudeFamily) thinkingOf(req ModelRequest) (map[string]any, []Message, []string) {
	var cfg map[string]any
	msgs := req.Messages
	switch {
	case req.Thinking > 0 && f.thinking == "adaptive":
		cfg = map[string]any{"type": "adaptive"}
		if f.display != "" {
			cfg["display"] = f.display
		}
	case req.Thinking > 0:
		cfg = map[string]any{"type": "enabled", "budget_tokens": max(req.Thinking, minThinkingBudget)}
	case f.off != "":
		cfg = map[string]any{"type": f.off}
		msgs = withoutThinking(msgs)
	}
	if !f.binding || !hasThinking(msgs) {
		return cfg, msgs, nil
	}
	if cfg == nil {
		cfg = map[string]any{"type": "adaptive"}
	}
	cfg["block_binding"] = map[string]any{"prefix_mismatch_behavior": "drop_block"}
	return cfg, msgs, []string{bindingBeta}
}

func hasThinking(msgs []Message) bool {
	for _, m := range msgs {
		for _, b := range m.Content {
			if isReasoning(b) {
				return true
			}
		}
	}
	return false
}

// isReasoning is a block the API can take back: one it signed, or a redacted one. Reasoning that came
// from another provider has no signature and is rejected if sent.
func isReasoning(b Block) bool {
	return b.Type == "thinking" && b.Signature != "" || b.Type == "redacted_thinking" && b.Data != ""
}

// withoutThinking removes every reasoning block of the history. An assistant message left empty is
// dropped by anthropicMessages.
func withoutThinking(msgs []Message) []Message {
	out := make([]Message, 0, len(msgs))
	for _, m := range msgs {
		kept := make([]Block, 0, len(m.Content))
		for _, b := range m.Content {
			if b.Type != "thinking" && b.Type != "redacted_thinking" {
				kept = append(kept, b)
			}
		}
		m.Content = kept
		out = append(out, m)
	}
	return out
}

// strictTools marks the tools strict: the arguments then always match the schema, which is what the
// forced call guaranteed.
func strictTools(tools []ToolSpec) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		var schema any
		_ = json.Unmarshal(t.InputSchema, &schema)
		closeObjects(schema)
		out = append(out, map[string]any{"name": t.Name, "description": t.Description, "input_schema": schema, "strict": true})
	}
	return out
}

// closeObjects adds what strict tool use needs to every object of a schema: additionalProperties false
// and a required list.
func closeObjects(node any) {
	switch n := node.(type) {
	case []any:
		for _, v := range n {
			closeObjects(v)
		}
	case map[string]any:
		if props, ok := n["properties"].(map[string]any); ok {
			n["additionalProperties"] = false
			if _, has := n["required"]; !has {
				names := make([]string, 0, len(props))
				for k := range props {
					names = append(names, k)
				}
				n["required"] = names
			}
		}
		for _, v := range n {
			closeObjects(v)
		}
	}
}

func anthropicMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		content := make([]map[string]any, 0, len(m.Content))
		for _, b := range m.Content {
			if wire := anthropicBlock(b); wire != nil {
				content = append(content, wire)
			}
		}
		if len(content) == 0 {
			continue
		}
		out = append(out, map[string]any{"role": m.Role, "content": content})
	}
	return out
}

func anthropicBlock(b Block) map[string]any {
	switch b.Type {
	case "text":
		if b.Text != "" {
			return map[string]any{"type": "text", "text": b.Text}
		}
	case "tool_use":
		in := b.Input
		if len(in) == 0 {
			in = json.RawMessage("{}")
		}
		return map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": in}
	case "tool_result":
		return map[string]any{"type": "tool_result", "tool_use_id": b.ToolUseID, "content": b.Content, "is_error": b.IsError}
	case "image":
		return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": b.MediaType, "data": b.Data}}
	case "thinking": // back unchanged, an empty text included; without a signature it is not Anthropic's
		if isReasoning(b) {
			return map[string]any{"type": "thinking", "thinking": b.Thinking, "signature": b.Signature}
		}
	case "redacted_thinking":
		if isReasoning(b) {
			return map[string]any{"type": "redacted_thinking", "data": b.Data}
		}
	}
	return nil
}
