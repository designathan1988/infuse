package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Model providers: the wire protocol of each service.
// Ollama adapter.

func (p *ollamaProvider) Name() string { return "ollama" }

func (p *ollamaProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	body := map[string]any{"model": req.Model, "stream": true, "messages": ollamaMessages(req.System, req.Messages)}
	if req.MaxTokens > 0 {
		body["options"] = map[string]any{"num_predict": req.MaxTokens}
	}
	if req.ForceTool != "" { // Ollama has no forced call: the answer is constrained to the schema instead
		for _, t := range req.Tools {
			if t.Name == req.ForceTool {
				body["format"] = t.InputSchema
			}
		}
	} else if len(req.Tools) > 0 {
		var ts []map[string]any
		for _, t := range req.Tools {
			ts = append(ts, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.InputSchema}})
		}
		body["tools"] = ts
	}
	resp, err := postJSON(ctx, p.client, p.base+"/api/chat", nil, body)
	if err != nil {
		return ModelResult{}, fmt.Errorf("Ollama não respondeu em %s: %w", p.base, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, httpError("Ollama", resp)
	}
	var text strings.Builder
	var calls []Block
	var res ModelResult
	sc := sseScanner(resp.Body)
	for sc.Scan() {
		var ev struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			Done            bool   `json:"done"`
			PromptEvalCount int    `json:"prompt_eval_count"`
			EvalCount       int    `json:"eval_count"`
			Error           string `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &ev) != nil {
			continue
		}
		if ev.Error != "" {
			return res, errors.New(ev.Error)
		}
		if ev.Message.Content != "" {
			text.WriteString(ev.Message.Content)
			emit(StreamEvent{Kind: "text", Text: ev.Message.Content})
		}
		for _, tc := range ev.Message.ToolCalls {
			id := fmt.Sprintf("o%d", len(calls)+1)
			args := tc.Function.Arguments
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			calls = append(calls, Block{Type: "tool_use", ID: id, Name: tc.Function.Name, Input: args})
			emit(StreamEvent{Kind: "tool_start", ID: id, Name: tc.Function.Name})
			emit(StreamEvent{Kind: "tool_input", ID: id, Text: string(args)})
			emit(StreamEvent{Kind: "tool_end", ID: id, Name: tc.Function.Name})
		}
		if ev.Done {
			res.Usage = Usage{Input: ev.PromptEvalCount, Output: ev.EvalCount}
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	if text.Len() > 0 {
		res.Content = append(res.Content, Block{Type: "text", Text: text.String()})
	}
	res.Content = append(res.Content, calls...)
	if len(calls) > 0 {
		res.StopReason = "tool_use"
	} else {
		res.StopReason = "end_turn"
	}
	return res, nil
}

func ollamaMessages(system string, msgs []Message) []map[string]any {
	out := []map[string]any{}
	if system != "" {
		out = append(out, map[string]any{"role": "system", "content": system})
	}
	for _, m := range msgs {
		if m.Role == "assistant" {
			am := map[string]any{"role": "assistant"}
			var texts []string
			var tcs []map[string]any
			for _, b := range m.Content {
				switch b.Type {
				case "text":
					texts = append(texts, b.Text)
				case "tool_use":
					args := map[string]any{}
					_ = json.Unmarshal(b.Input, &args)
					tcs = append(tcs, map[string]any{"function": map[string]any{"name": b.Name, "arguments": args}})
				}
			}
			am["content"] = strings.Join(texts, "\n")
			if len(tcs) > 0 {
				am["tool_calls"] = tcs
			}
			out = append(out, am)
			continue
		}
		var texts, images []string
		for _, b := range m.Content {
			switch b.Type {
			case "tool_result":
				out = append(out, map[string]any{"role": "tool", "content": b.Content})
			case "text":
				texts = append(texts, b.Text)
			case "image":
				images = append(images, b.Data)
			}
		}
		if len(texts) > 0 || len(images) > 0 {
			um := map[string]any{"role": "user", "content": strings.Join(texts, "\n")}
			if len(images) > 0 {
				um["images"] = images
			}
			out = append(out, um)
		}
	}
	return out
}

// ---------------------------------------------------------------- model listing

// ListModels asks the provider for its models (Ollama: installed ones). Used by the Providers screen.
