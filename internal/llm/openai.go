package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
)

// Model providers: the wire protocol of each service.
// OpenAI-compatible and DeepSeek adapters.

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	headers := map[string]string{}
	if p.key != "" {
		headers["authorization"] = "Bearer " + p.key
	}
	resp, err := postJSON(ctx, p.client, p.base+"/v1/chat/completions", headers, p.body(req))
	if err != nil {
		return ModelResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, httpError("OpenAI", resp)
	}
	return readOpenAIStream(resp.Body, emit)
}

func (p *openAIProvider) body(req ModelRequest) map[string]any {
	ds := strings.Contains(p.base, "deepseek.com")
	body := map[string]any{"model": req.Model, "stream": true, "stream_options": map[string]any{"include_usage": true}, "messages": openAIMessages(req.System, req.Messages, ds)}
	if ds {
		deepseekThinking(body, req)
	}
	if req.MaxTokens > 0 {
		body["max_completion_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		var ts []map[string]any
		for _, t := range req.Tools {
			ts = append(ts, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.InputSchema}})
		}
		body["tools"] = ts
	}
	if req.ForceTool != "" {
		body["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": req.ForceTool}}
	}
	return body
}

type openAICall struct {
	id, name string
	args     strings.Builder
}

type openAIStream struct {
	res       ModelResult
	calls     map[int]*openAICall
	text      strings.Builder
	reasoning strings.Builder
}

func readOpenAIStream(r io.Reader, emit func(StreamEvent)) (ModelResult, error) {
	a := &openAIStream{calls: map[int]*openAICall{}}
	sc := sseScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}
		if err := a.event(data, emit); err != nil {
			return a.res, err
		}
	}
	if err := sc.Err(); err != nil {
		return a.res, err
	}
	a.content(emit)
	return a.res, nil
}

func (a *openAIStream) event(data []byte, emit func(StreamEvent)) error {
	var ev struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Delta        struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"` // DeepSeek reasoner
				Reasoning        string `json:"reasoning"`         // OpenRouter
				ToolCalls        []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
		Usage *struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			PromptDetails    struct {
				Cached int `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
		Error *struct{ Message string } `json:"error"`
	}
	if json.Unmarshal(data, &ev) != nil {
		return nil
	}
	if ev.Error != nil {
		return errors.New(ev.Error.Message)
	}
	if ev.Usage != nil {
		a.res.Usage = Usage{Input: ev.Usage.PromptTokens - ev.Usage.PromptDetails.Cached, Output: ev.Usage.CompletionTokens, CacheRead: ev.Usage.PromptDetails.Cached}
	}
	if len(ev.Choices) == 0 {
		return nil
	}
	d := ev.Choices[0].Delta
	if r := d.ReasoningContent + d.Reasoning; r != "" {
		a.reasoning.WriteString(r)
		emit(StreamEvent{Kind: "thinking", Text: r})
	}
	if d.Content != "" {
		a.text.WriteString(d.Content)
		emit(StreamEvent{Kind: "text", Text: d.Content})
	}
	for _, tc := range d.ToolCalls {
		a.toolDelta(tc.Index, tc.ID, tc.Function.Name, tc.Function.Arguments, emit)
	}
	if ev.Choices[0].FinishReason != "" {
		a.res.StopReason = ev.Choices[0].FinishReason
	}
	return nil
}

func (a *openAIStream) toolDelta(index int, id, name, args string, emit func(StreamEvent)) {
	c := a.calls[index]
	if c == nil {
		c = &openAICall{}
		a.calls[index] = c
	}
	if id != "" {
		c.id = id
	}
	if name != "" {
		c.name = name
		emit(StreamEvent{Kind: "tool_start", ID: c.id, Name: c.name, Index: index})
	}
	if args != "" {
		c.args.WriteString(args)
		emit(StreamEvent{Kind: "tool_input", ID: c.id, Text: args, Index: index})
	}
}

func (a *openAIStream) content(emit func(StreamEvent)) {
	res := &a.res
	if a.reasoning.Len() > 0 { // kept with the message: DeepSeek needs it back in later requests (openAIMessages)
		res.Content = append(res.Content, Block{Type: "thinking", Thinking: a.reasoning.String()})
	}
	if a.text.Len() > 0 {
		res.Content = append(res.Content, Block{Type: "text", Text: a.text.String()})
	}
	idx := make([]int, 0, len(a.calls))
	for i := range a.calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		c := a.calls[i]
		args := strings.TrimSpace(c.args.String())
		if args == "" {
			args = "{}"
		}
		res.Content = append(res.Content, Block{Type: "tool_use", ID: c.id, Name: c.name, Input: json.RawMessage(args)})
		emit(StreamEvent{Kind: "tool_end", ID: c.id, Name: c.name, Index: i})
	}
	if len(a.calls) > 0 {
		if res.StopReason != "length" { // cut at the limit: the calls may be half-written (see truncatedReply)
			res.StopReason = "tool_use"
		}
	} else if res.StopReason == "" || res.StopReason == "stop" {
		res.StopReason = "end_turn"
	}
}

// DeepSeek thinking mode (https://api-docs.deepseek.com/guides/thinking_mode, read 2026-10-31):
// thinking is switched with {"thinking":{"type":"enabled|disabled"}} and its depth with reasoning_effort
// (low | high | max); with tools in the request, the reasoning_content of every earlier assistant
// message must be sent back, or the API answers 400. The legacy names (deepseek-chat, deepseek-reasoner,
// deepseek-v4-flash) are served by deepseek-flash (https://api-docs.deepseek.com/quick_start/pricing).
func deepseekThinking(body map[string]any, req ModelRequest) {
	switch req.Model {
	case "deepseek-chat", "deepseek-reasoner", "deepseek-v4-flash":
		body["model"] = "deepseek-flash"
	}
	if req.Thinking <= 0 {
		body["thinking"] = map[string]any{"type": "disabled"}
		return
	}
	body["thinking"] = map[string]any{"type": "enabled"}
	switch {
	case req.Thinking <= 4000:
		body["reasoning_effort"] = "low"
	case req.Thinking <= 12000:
		body["reasoning_effort"] = "high"
	default:
		body["reasoning_effort"] = "max"
	}
}

func openAIMessages(system string, msgs []Message, withReasoning bool) []map[string]any {
	out := []map[string]any{}
	if system != "" {
		out = append(out, map[string]any{"role": "system", "content": system})
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			am := map[string]any{"role": "assistant"}
			var texts, thoughts []string
			var tcs []map[string]any
			for _, b := range m.Content {
				switch b.Type {
				case "thinking":
					thoughts = append(thoughts, b.Thinking)
				case "text":
					texts = append(texts, b.Text)
				case "tool_use":
					args := string(b.Input)
					if args == "" {
						args = "{}"
					}
					tcs = append(tcs, map[string]any{"id": b.ID, "type": "function", "function": map[string]any{"name": b.Name, "arguments": args}})
				}
			}
			am["content"] = strings.Join(texts, "\n")
			if withReasoning { // DeepSeek: the reasoning goes back with its message (empty when there was none)
				am["reasoning_content"] = strings.Join(thoughts, "\n")
			}
			if len(tcs) > 0 {
				am["tool_calls"] = tcs
			}
			out = append(out, am)
			continue
		}
		var parts []map[string]any
		for _, b := range m.Content {
			switch b.Type {
			case "tool_result":
				out = append(out, map[string]any{"role": "tool", "tool_call_id": b.ToolUseID, "content": b.Content})
			case "text":
				parts = append(parts, map[string]any{"type": "text", "text": b.Text})
			case "image":
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + b.MediaType + ";base64," + b.Data}})
			}
		}
		if len(parts) > 0 {
			out = append(out, map[string]any{"role": "user", "content": parts})
		}
	}
	return out
}

// ---------------------------------------------------------------- Google Gemini

type googleProvider struct {
	key, base string
	client    *http.Client
}
