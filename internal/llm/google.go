package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Model providers: the wire protocol of each service.
// Google Gemini adapter.

func (p *googleProvider) Name() string { return "google" }

func (p *googleProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	if p.key == "" {
		return ModelResult{}, errors.New("chave de API do Google não configurada")
	}
	body := map[string]any{"contents": googleContents(req.Messages)}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []map[string]any{{"text": req.System}}}
	}
	gen := map[string]any{}
	if req.MaxTokens > 0 {
		gen["maxOutputTokens"] = req.MaxTokens
	}
	if req.Thinking > 0 {
		gen["thinkingConfig"] = map[string]any{"thinkingBudget": req.Thinking, "includeThoughts": true}
	}
	if len(gen) > 0 {
		body["generationConfig"] = gen
	}
	if len(req.Tools) > 0 {
		var fds []map[string]any
		for _, t := range req.Tools {
			fds = append(fds, map[string]any{"name": t.Name, "description": t.Description, "parameters": geminiSchema(t.InputSchema)})
		}
		body["tools"] = []map[string]any{{"functionDeclarations": fds}}
	}
	if req.ForceTool != "" {
		body["toolConfig"] = map[string]any{"functionCallingConfig": map[string]any{"mode": "ANY", "allowedFunctionNames": []string{req.ForceTool}}}
	}
	url := p.base + "/v1beta/models/" + req.Model + ":streamGenerateContent?alt=sse"
	resp, err := postJSON(ctx, p.client, url, map[string]string{"x-goog-api-key": p.key}, body)
	if err != nil {
		return ModelResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, httpError("Gemini", resp)
	}
	var text, thought strings.Builder
	var calls []Block
	var res ModelResult
	sc := sseScanner(resp.Body)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		var ev struct {
			Candidates []struct {
				FinishReason string `json:"finishReason"`
				Content      struct {
					Parts []struct {
						Text         string `json:"text"`
						Thought      bool   `json:"thought"`
						FunctionCall *struct {
							Name string          `json:"name"`
							Args json.RawMessage `json:"args"`
						} `json:"functionCall"`
					} `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
			UsageMetadata *struct {
				PromptTokenCount        int `json:"promptTokenCount"`
				CandidatesTokenCount    int `json:"candidatesTokenCount"`
				ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
				CachedContentTokenCount int `json:"cachedContentTokenCount"`
			} `json:"usageMetadata"`
			Error *struct{ Message string } `json:"error"`
		}
		if json.Unmarshal(bytes.TrimSpace(line[5:]), &ev) != nil {
			continue
		}
		if ev.Error != nil {
			return res, errors.New(ev.Error.Message)
		}
		if u := ev.UsageMetadata; u != nil {
			res.Usage = Usage{Input: u.PromptTokenCount - u.CachedContentTokenCount, Output: u.CandidatesTokenCount + u.ThoughtsTokenCount, CacheRead: u.CachedContentTokenCount}
		}
		for _, c := range ev.Candidates {
			for _, part := range c.Content.Parts {
				switch {
				case part.FunctionCall != nil:
					id := fmt.Sprintf("g%d", len(calls)+1)
					args := part.FunctionCall.Args
					if len(args) == 0 {
						args = json.RawMessage("{}")
					}
					calls = append(calls, Block{Type: "tool_use", ID: id, Name: part.FunctionCall.Name, Input: args})
					emit(StreamEvent{Kind: "tool_start", ID: id, Name: part.FunctionCall.Name})
					emit(StreamEvent{Kind: "tool_input", ID: id, Text: string(args)})
					emit(StreamEvent{Kind: "tool_end", ID: id, Name: part.FunctionCall.Name})
				case part.Thought:
					thought.WriteString(part.Text)
					emit(StreamEvent{Kind: "thinking", Text: part.Text})
				case part.Text != "":
					text.WriteString(part.Text)
					emit(StreamEvent{Kind: "text", Text: part.Text})
				}
			}
			if c.FinishReason != "" {
				res.StopReason = strings.ToLower(c.FinishReason)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	if thought.Len() > 0 {
		res.Content = append(res.Content, Block{Type: "thinking", Thinking: thought.String()})
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

// geminiSchema drops JSON-Schema keywords Gemini rejects.
func geminiSchema(raw json.RawMessage) any {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return map[string]any{"type": "object"}
	}
	var clean func(any) any
	clean = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, val := range t {
				if k == "additionalProperties" || k == "$schema" || k == "default" {
					continue
				}
				out[k] = clean(val)
			}
			return out
		case []any:
			for i := range t {
				t[i] = clean(t[i])
			}
		}
		return x
	}
	return clean(v)
}

func googleContents(msgs []Message) []map[string]any {
	var out []map[string]any
	names := map[string]string{} // tool_use id → function name (functionResponse needs the name)
	for _, m := range msgs {
		role := "user"
		if m.Role == "assistant" {
			role = "model"
		}
		var parts []map[string]any
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if b.Text != "" {
					parts = append(parts, map[string]any{"text": b.Text})
				}
			case "image":
				parts = append(parts, map[string]any{"inlineData": map[string]any{"mimeType": b.MediaType, "data": b.Data}})
			case "tool_use":
				names[b.ID] = b.Name
				args := map[string]any{}
				_ = json.Unmarshal(b.Input, &args)
				parts = append(parts, map[string]any{"functionCall": map[string]any{"name": b.Name, "args": args}})
			case "tool_result":
				parts = append(parts, map[string]any{"functionResponse": map[string]any{"name": names[b.ToolUseID], "response": map[string]any{"output": b.Content, "error": b.IsError}}})
			}
		}
		if len(parts) > 0 {
			out = append(out, map[string]any{"role": role, "parts": parts})
		}
	}
	return out
}

// ---------------------------------------------------------------- Ollama

type ollamaProvider struct {
	base   string
	client *http.Client
}
