package llm

// Model providers. The agent loop talks to this interface only; providers are registered by name.
// The Anthropic Messages API lives in the anthropic*.go files; the mock provider in mock.go.

import (
	"context"
	"fmt"
)

type ModelRequest struct {
	Model     string
	System    string
	Messages  []Message
	Tools     []ToolSpec
	MaxTokens int
	Thinking  int    // budget tokens; 0 = off
	Effort    string // low | medium | high | max; "" = the model's default
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
	APIKey  string   `json:"apiKey"`
	BaseURL string   `json:"baseUrl,omitempty"`
	Wire    WireCaps `json:"-"` // what the preset's endpoint accepts (internal/data/providers.json)
}

// WireCaps is what an OpenAI-compatible endpoint accepts, where presets differ. The zero value is
// OpenAI's own shape.
type WireCaps struct {
	MaxTokensField string // max_completion_tokens (default) or max_tokens
	NoStreamUsage  bool   // the endpoint rejects or ignores stream_options
}

func NewProvider(name string, cfg ProviderConfig) (Provider, error) {
	mk, ok := providers[name]
	if !ok {
		return nil, fmt.Errorf("provedor desconhecido: %s", name)
	}
	return mk(cfg), nil
}

func init() {
	RegisterProvider("mock", func(cfg ProviderConfig) Provider { return &mockProvider{} })
}
