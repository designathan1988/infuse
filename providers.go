package main

// OpenAI (Chat Completions, SSE, function calling; any compatible endpoint via BaseURL: LM Studio, vLLM,
// OpenRouter, Groq…), Google Gemini (streamGenerateContent?alt=sse, functionDeclarations) and Ollama
// (/api/chat NDJSON, no key). Each is registered by name; the loop only knows the Provider interface.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	openAIDefaultBase = "https://api.openai.com"
	googleDefaultBase = "https://generativelanguage.googleapis.com"
	ollamaDefaultBase = "http://127.0.0.1:11434"
)

func init() {
	registerProvider("openai", func(cfg ProviderConfig) Provider {
		return &openAIProvider{key: cfg.APIKey, base: orDefault(cfg.BaseURL, openAIDefaultBase), client: httpClient()}
	})
	registerProvider("google", func(cfg ProviderConfig) Provider {
		return &googleProvider{key: cfg.APIKey, base: orDefault(cfg.BaseURL, googleDefaultBase), client: httpClient()}
	})
	registerProvider("ollama", func(cfg ProviderConfig) Provider {
		return &ollamaProvider{base: orDefault(cfg.BaseURL, ollamaDefaultBase), client: httpClient()}
	})
}

func httpClient() *http.Client { return &http.Client{Timeout: 10 * time.Minute} }
func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return strings.TrimRight(v, "/")
}

func httpError(provider string, resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Error.Message != "" {
		return fmt.Errorf("%s %d: %s", provider, resp.StatusCode, e.Error.Message)
	}
	return fmt.Errorf("%s %d: %s", provider, resp.StatusCode, strings.TrimSpace(string(raw)))
}

func postJSON(ctx context.Context, client *http.Client, url string, headers map[string]string, body any) (*http.Response, error) {
	b, _ := json.Marshal(body)
	return doWithRetry(ctx, client, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("content-type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return req, nil
	})
}

// doWithRetry retries rate-limit and overload answers (429, 503, 529) and dropped connections up to
// three times, waiting Retry-After when the provider sends it, else 1s, 2s, 4s.
func doWithRetry(ctx context.Context, client *http.Client, newReq func() (*http.Request, error)) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		req, err := newReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode != 429 && resp.StatusCode != 503 && resp.StatusCode != 529 {
			return resp, nil
		}
		wait := time.Duration(1<<attempt) * time.Second
		if err == nil {
			if s, perr := strconv.Atoi(resp.Header.Get("Retry-After")); perr == nil && s > 0 && s <= 60 {
				wait = time.Duration(s) * time.Second
			}
			if attempt == 3 {
				return resp, nil // give the caller the provider's own error
			}
			resp.Body.Close()
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			if ctx.Err() != nil {
				return nil, err
			}
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return nil, lastErr
}

func sseScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	return sc
}

// ---------------------------------------------------------------- OpenAI

type openAIProvider struct {
	key, base string
	client    *http.Client
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	body := map[string]any{"model": req.Model, "stream": true, "stream_options": map[string]any{"include_usage": true}, "messages": openAIMessages(req.System, req.Messages)}
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
	headers := map[string]string{}
	if p.key != "" {
		headers["authorization"] = "Bearer " + p.key
	}
	resp, err := postJSON(ctx, p.client, p.base+"/v1/chat/completions", headers, body)
	if err != nil {
		return ModelResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, httpError("OpenAI", resp)
	}
	type call struct {
		id, name string
		args     strings.Builder
	}
	calls := map[int]*call{}
	var text, reasoning strings.Builder
	var res ModelResult
	sc := sseScanner(resp.Body)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.HasPrefix(line, []byte("data:")) {
			continue
		}
		data := bytes.TrimSpace(line[5:])
		if bytes.Equal(data, []byte("[DONE]")) {
			break
		}
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
			continue
		}
		if ev.Error != nil {
			return res, errors.New(ev.Error.Message)
		}
		if ev.Usage != nil {
			res.Usage = Usage{Input: ev.Usage.PromptTokens - ev.Usage.PromptDetails.Cached, Output: ev.Usage.CompletionTokens, CacheRead: ev.Usage.PromptDetails.Cached}
		}
		if len(ev.Choices) == 0 {
			continue
		}
		d := ev.Choices[0].Delta
		if r := d.ReasoningContent + d.Reasoning; r != "" {
			reasoning.WriteString(r)
			emit(StreamEvent{Kind: "thinking", Text: r})
		}
		if d.Content != "" {
			text.WriteString(d.Content)
			emit(StreamEvent{Kind: "text", Text: d.Content})
		}
		for _, tc := range d.ToolCalls {
			c := calls[tc.Index]
			if c == nil {
				c = &call{}
				calls[tc.Index] = c
			}
			if tc.ID != "" {
				c.id = tc.ID
			}
			if tc.Function.Name != "" {
				c.name = tc.Function.Name
				emit(StreamEvent{Kind: "tool_start", ID: c.id, Name: c.name, Index: tc.Index})
			}
			if tc.Function.Arguments != "" {
				c.args.WriteString(tc.Function.Arguments)
				emit(StreamEvent{Kind: "tool_input", ID: c.id, Text: tc.Function.Arguments, Index: tc.Index})
			}
		}
		if ev.Choices[0].FinishReason != "" {
			res.StopReason = ev.Choices[0].FinishReason
		}
	}
	if err := sc.Err(); err != nil {
		return res, err
	}
	if reasoning.Len() > 0 { // kept for the transcript; never sent back (openAIMessages skips thinking)
		res.Content = append(res.Content, Block{Type: "thinking", Thinking: reasoning.String()})
	}
	if text.Len() > 0 {
		res.Content = append(res.Content, Block{Type: "text", Text: text.String()})
	}
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		c := calls[i]
		args := strings.TrimSpace(c.args.String())
		if args == "" {
			args = "{}"
		}
		res.Content = append(res.Content, Block{Type: "tool_use", ID: c.id, Name: c.name, Input: json.RawMessage(args)})
		emit(StreamEvent{Kind: "tool_end", ID: c.id, Name: c.name, Index: i})
	}
	if len(calls) > 0 {
		res.StopReason = "tool_use"
	} else if res.StopReason == "" || res.StopReason == "stop" {
		res.StopReason = "end_turn"
	}
	return res, nil
}

func openAIMessages(system string, msgs []Message) []map[string]any {
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
					args := string(b.Input)
					if args == "" {
						args = "{}"
					}
					tcs = append(tcs, map[string]any{"id": b.ID, "type": "function", "function": map[string]any{"name": b.Name, "arguments": args}})
				}
			}
			am["content"] = strings.Join(texts, "\n")
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

func (p *ollamaProvider) Name() string { return "ollama" }

func (p *ollamaProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	body := map[string]any{"model": req.Model, "stream": true, "messages": ollamaMessages(req.System, req.Messages)}
	if req.MaxTokens > 0 {
		body["options"] = map[string]any{"num_predict": req.MaxTokens}
	}
	if len(req.Tools) > 0 {
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
func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	get := func(url string, headers map[string]string) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, httpError(name, resp)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	}
	var names []string
	switch name {
	case "anthropic":
		b, err := get(orDefault(cfg.BaseURL, "https://api.anthropic.com")+"/v1/models?limit=100", map[string]string{"x-api-key": cfg.APIKey, "anthropic-version": "2023-06-01"})
		if err != nil {
			return nil, err
		}
		var r struct {
			Data []struct{ ID string } `json:"data"`
		}
		_ = json.Unmarshal(b, &r)
		for _, d := range r.Data {
			names = append(names, d.ID)
		}
	case "openai":
		h := map[string]string{}
		if cfg.APIKey != "" {
			h["authorization"] = "Bearer " + cfg.APIKey
		}
		b, err := get(orDefault(cfg.BaseURL, openAIDefaultBase)+"/v1/models", h)
		if err != nil {
			return nil, err
		}
		var r struct {
			Data []struct{ ID string } `json:"data"`
		}
		_ = json.Unmarshal(b, &r)
		for _, d := range r.Data {
			names = append(names, d.ID)
		}
	case "google":
		b, err := get(orDefault(cfg.BaseURL, googleDefaultBase)+"/v1beta/models?pageSize=200", map[string]string{"x-goog-api-key": cfg.APIKey})
		if err != nil {
			return nil, err
		}
		var r struct {
			Models []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		_ = json.Unmarshal(b, &r)
		for _, m := range r.Models {
			for _, me := range m.Methods {
				if me == "generateContent" {
					names = append(names, strings.TrimPrefix(m.Name, "models/"))
					break
				}
			}
		}
	case "ollama":
		b, err := get(orDefault(cfg.BaseURL, ollamaDefaultBase)+"/api/tags", nil)
		if err != nil {
			return nil, fmt.Errorf("Ollama não respondeu (está rodando?): %w", err)
		}
		var r struct {
			Models []struct{ Name string } `json:"models"`
		}
		_ = json.Unmarshal(b, &r)
		for _, m := range r.Models {
			names = append(names, m.Name)
		}
	case "mock":
		names = []string{"mock"}
	default:
		return nil, fmt.Errorf("provedor desconhecido: %s", name)
	}
	sort.Strings(names)
	return names, nil
}
