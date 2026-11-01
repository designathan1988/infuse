package llm

// Model providers. The agent loop talks to this interface only; providers are registered by name.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/core"
	"io"
	"net/http"
	"strings"
	"time"
)

type ModelRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolSpec
	MaxTokens int
	Thinking  int    // budget tokens; 0 = off
	ForceTool string // the model must answer by calling this tool (askjson.go); needs Thinking 0
}

type ModelResult struct {
	Content    []Block
	StopReason string
	Usage      Usage
}

// StreamEvent is what the loop forwards to the UI while the model answers.
type StreamEvent struct {
	Kind  string `json:"kind"` // text | thinking | tool_start | tool_input | tool_end
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Index int    `json:"index"`
}

type Provider interface {
	Name() string
	Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error)
}

var providers = map[string]func(cfg ProviderConfig) Provider{}

func RegisterProvider(name string, mk func(cfg ProviderConfig) Provider) { providers[name] = mk }

type ProviderConfig struct {
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseUrl,omitempty"`
}

func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	mk, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("provedor desconhecido: %s", name)
	}
	return mk(cfg), nil
}

// ---------------------------------------------------------------- Anthropic Messages API

type anthropicProvider struct {
	key, base string
	client    *http.Client
}

func init() {
	RegisterProvider("anthropic", func(cfg ProviderConfig) Provider {
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return &anthropicProvider{key: cfg.APIKey, base: strings.TrimRight(base, "/"), client: &http.Client{Timeout: 10 * time.Minute}}
	})
	RegisterProvider("mock", func(cfg ProviderConfig) Provider { return &mockProvider{} })
}

func (p *anthropicProvider) Name() string { return "anthropic" }

func (p *anthropicProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	if p.key == "" {
		return ModelResult{}, errors.New("chave de API da Anthropic não configurada (Configurações)")
	}
	resp, err := postJSON(ctx, p.client, p.base+"/v1/messages", map[string]string{"x-api-key": p.key, "anthropic-version": "2023-06-01"}, p.body(req))
	if err != nil {
		return ModelResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, apiError(resp)
	}
	return readAnthropicStream(resp.Body, emit)
}

func (p *anthropicProvider) body(req ModelRequest) map[string]any {
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": req.MaxTokens,
		"messages":   anthropicMessages(req.Messages),
		"stream":     true,
	}
	if req.System != "" {
		body["system"] = []map[string]any{{"type": "text", "text": req.System, "cache_control": map[string]string{"type": "ephemeral"}}}
	}
	if len(req.Tools) > 0 {
		body["tools"] = req.Tools
	}
	if req.Thinking > 0 {
		body["thinking"] = map[string]any{"type": "enabled", "budget_tokens": req.Thinking}
	}
	if req.ForceTool != "" {
		body["tool_choice"] = map[string]any{"type": "tool", "name": req.ForceTool}
	}
	return body
}

// apiError turns a failed HTTP response into the provider's own message when it has one.
func apiError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var e struct {
		Error struct{ Type, Message string } `json:"error"`
	}
	_ = json.Unmarshal(raw, &e)
	if e.Error.Message != "" {
		return fmt.Errorf("API %d: %s", resp.StatusCode, e.Error.Message)
	}
	return fmt.Errorf("API %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
}

type anthropicStream struct {
	res      ModelResult
	blocks   map[int]*Block
	inputBuf map[int]*strings.Builder
	order    []int
}

func readAnthropicStream(r io.Reader, emit func(StreamEvent)) (ModelResult, error) {
	a := &anthropicStream{blocks: map[int]*Block{}, inputBuf: map[int]*strings.Builder{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	var eventName string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if err := a.event(eventName, []byte(strings.TrimPrefix(line, "data: ")), emit); err != nil {
			return a.res, err
		}
	}
	if err := sc.Err(); err != nil {
		return a.res, err
	}
	a.content()
	return a.res, nil
}

func (a *anthropicStream) event(name string, data []byte, emit func(StreamEvent)) error {
	switch name {
	case "message_start":
		var ev struct {
			Message struct {
				Usage struct {
					InputTokens              int `json:"input_tokens"`
					CacheReadInputTokens     int `json:"cache_read_input_tokens"`
					CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		_ = json.Unmarshal(data, &ev)
		a.res.Usage.Input = ev.Message.Usage.InputTokens
		a.res.Usage.CacheRead = ev.Message.Usage.CacheReadInputTokens
		a.res.Usage.CacheWrite = ev.Message.Usage.CacheCreationInputTokens
	case "content_block_start":
		a.startBlock(data, emit)
	case "content_block_delta":
		a.delta(data, emit)
	case "content_block_stop":
		var ev struct {
			Index int `json:"index"`
		}
		_ = json.Unmarshal(data, &ev)
		if blk := a.blocks[ev.Index]; blk != nil && blk.Type == "tool_use" {
			raw := a.inputBuf[ev.Index].String()
			if strings.TrimSpace(raw) == "" {
				raw = "{}"
			}
			blk.Input = json.RawMessage(raw)
			emit(StreamEvent{Kind: "tool_end", ID: blk.ID, Name: blk.Name, Index: ev.Index})
		}
	case "message_delta":
		var ev struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(data, &ev)
		a.res.StopReason = ev.Delta.StopReason
		a.res.Usage.Output = ev.Usage.OutputTokens
	case "error":
		var ev struct {
			Error struct{ Type, Message string } `json:"error"`
		}
		_ = json.Unmarshal(data, &ev)
		return fmt.Errorf("stream: %s", ev.Error.Message)
	}
	return nil
}

func (a *anthropicStream) startBlock(data []byte, emit func(StreamEvent)) {
	var ev struct {
		Index        int `json:"index"`
		ContentBlock struct {
			Type string `json:"type"`
			ID   string `json:"id"`
			Name string `json:"name"`
			Text string `json:"text"`
		} `json:"content_block"`
	}
	_ = json.Unmarshal(data, &ev)
	blk := &Block{Type: ev.ContentBlock.Type, ID: ev.ContentBlock.ID, Name: ev.ContentBlock.Name, Text: ev.ContentBlock.Text}
	a.blocks[ev.Index] = blk
	a.order = append(a.order, ev.Index)
	if blk.Type == "tool_use" {
		a.inputBuf[ev.Index] = &strings.Builder{}
		emit(StreamEvent{Kind: "tool_start", ID: blk.ID, Name: blk.Name, Index: ev.Index})
	}
}

func (a *anthropicStream) delta(data []byte, emit func(StreamEvent)) {
	var ev struct {
		Index int `json:"index"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
			Thinking    string `json:"thinking"`
		} `json:"delta"`
	}
	_ = json.Unmarshal(data, &ev)
	blk := a.blocks[ev.Index]
	if blk == nil {
		return
	}
	switch ev.Delta.Type {
	case "text_delta":
		blk.Text += ev.Delta.Text
		emit(StreamEvent{Kind: "text", Text: ev.Delta.Text, Index: ev.Index})
	case "input_json_delta":
		a.inputBuf[ev.Index].WriteString(ev.Delta.PartialJSON)
		emit(StreamEvent{Kind: "tool_input", Text: ev.Delta.PartialJSON, ID: blk.ID, Index: ev.Index})
	case "thinking_delta":
		blk.Thinking += ev.Delta.Thinking
		emit(StreamEvent{Kind: "thinking", Text: ev.Delta.Thinking, Index: ev.Index})
	}
}

func (a *anthropicStream) content() {
	for _, i := range a.order {
		blk := a.blocks[i]
		if blk.Type == "thinking" && blk.Thinking == "" {
			continue
		}
		a.res.Content = append(a.res.Content, *blk)
	}
}

func anthropicMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		content := make([]map[string]any, 0, len(m.Content))
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if b.Text == "" {
					continue
				}
				content = append(content, map[string]any{"type": "text", "text": b.Text})
			case "tool_use":
				in := b.Input
				if len(in) == 0 {
					in = json.RawMessage("{}")
				}
				content = append(content, map[string]any{"type": "tool_use", "id": b.ID, "name": b.Name, "input": in})
			case "tool_result":
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": b.ToolUseID, "content": b.Content, "is_error": b.IsError})
			case "image":
				content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": b.MediaType, "data": b.Data}})
			case "thinking":
				// thinking blocks are not sent back without signatures; drop them
			}
		}
		if len(content) == 0 {
			continue
		}
		out = append(out, map[string]any{"role": m.Role, "content": content})
	}
	return out
}

// ---------------------------------------------------------------- mock (for tests and demo)

// mockProvider plays a scripted agent: it reacts to the last user text with deterministic tool calls,
// so the whole loop (streaming, permissions, filter, checkpoints) can be exercised without a key.
type mockProvider struct{}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	if req.ForceTool != "" { // a forced structured answer (askJSON): the empty answer is valid and honest
		in := json.RawMessage("{}")
		emit(StreamEvent{Kind: "tool_start", ID: "tu_f", Name: req.ForceTool})
		emit(StreamEvent{Kind: "tool_input", ID: "tu_f", Text: "{}"})
		emit(StreamEvent{Kind: "tool_end", ID: "tu_f", Name: req.ForceTool})
		return ModelResult{Content: []Block{{Type: "tool_use", ID: "tu_f", Name: req.ForceTool, Input: in}}, StopReason: "tool_use", Usage: Usage{Input: 300, Output: 5}}, nil
	}
	last := req.Messages[len(req.Messages)-1]
	say := func(s string) (ModelResult, error) {
		for _, ch := range strings.SplitAfter(s, " ") {
			emit(StreamEvent{Kind: "text", Text: ch})
			time.Sleep(8 * time.Millisecond)
		}
		return ModelResult{Content: []Block{{Type: "text", Text: s}}, StopReason: "end_turn", Usage: Usage{Input: 1200, Output: len(s) / 4}}, nil
	}
	call := func(id, name string, input any) (ModelResult, error) {
		in, _ := json.Marshal(input)
		emit(StreamEvent{Kind: "tool_start", ID: id, Name: name})
		emit(StreamEvent{Kind: "tool_input", ID: id, Text: string(in)})
		emit(StreamEvent{Kind: "tool_end", ID: id, Name: name})
		return ModelResult{Content: []Block{{Type: "text", Text: "Vou fazer isso."}, {Type: "tool_use", ID: id, Name: name, Input: in}}, StopReason: "tool_use", Usage: Usage{Input: 1500, Output: 60}}, nil
	}
	// Tool result just came back → answer.
	if last.Role == "user" && len(last.Content) > 0 && last.Content[0].Type == "tool_result" {
		r := last.Content[0]
		if strings.Contains(r.Content, "ls-step") {
			return call("tu_2", "Write", map[string]any{"path": "src/demo.ts", "content": "export const demo = () => `ok ${1}`;\n"})
		}
		return say("Concluído. O resultado da ferramenta foi: " + core.Truncate(strings.TrimSpace(r.Content), 160))
	}
	text := ""
	for _, b := range last.Content {
		if b.Type == "text" {
			text += b.Text
		}
	}
	switch {
	case strings.Contains(strings.ToLower(text), "demo"):
		return call("tu_1", "Bash", map[string]any{"command": "echo ls-step && ls"})
	case strings.Contains(strings.ToLower(text), "heredoc"):
		return call("tu_3", "Bash", map[string]any{"command": "cat > src/h.ts <<EOF\nexport const h = `x${y}`;\nEOF\necho feito"})
	case strings.Contains(strings.ToLower(text), "oracle") || strings.Contains(strings.ToLower(text), "oráculo"):
		return call("tu_4", "Write", map[string]any{"path": ".infuse/oracle/done.md", "content": "hack"})
	case strings.Contains(strings.ToLower(text), "pergunta"):
		return call("tu_5", "AskUserQuestion", map[string]any{"question": "Qual abordagem?", "options": []string{"A", "B"}})
	}
	return say("Olá. Sou o provedor de demonstração: peça “demo”, “heredoc”, “oráculo” ou “pergunta” para ver o filtro agindo.")
}
