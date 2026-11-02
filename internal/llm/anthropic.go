package llm

// The Anthropic Messages API: first-party, and through the clouds (anthropic_cloud.go), which serve the
// same request and the same stream behind their own address and credentials.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const anthropicVersion = "2023-06-01"

// anthropicAccess is where a request goes and how it proves who sends it.
type anthropicAccess interface {
	url(model string) string
	// credentials returns the headers of the request and, when the request must be signed, how.
	credentials(ctx context.Context) (map[string]string, signFunc, error)
	// adapt changes the body where the platform differs from the Messages API.
	adapt(body map[string]any)
}

type anthropicProvider struct {
	name   string
	access anthropicAccess
	client *http.Client
}

// firstParty is api.anthropic.com or a compatible base URL.
type firstParty struct{ key, base string }

func (a firstParty) url(string) string { return a.base + "/v1/messages" }
func (a firstParty) adapt(map[string]any) {
}

func (a firstParty) credentials(context.Context) (map[string]string, signFunc, error) {
	if a.key == "" {
		return nil, nil, errors.New("Anthropic API key not configured (Settings)")
	}
	return map[string]string{"x-api-key": a.key, "anthropic-version": anthropicVersion}, nil, nil
}

func init() {
	RegisterProvider("anthropic", func(cfg ProviderConfig) Provider {
		base := cfg.BaseURL
		if base == "" {
			base = "https://api.anthropic.com"
		}
		return &anthropicProvider{name: "anthropic", access: firstParty{cfg.APIKey, strings.TrimRight(base, "/")}, client: &http.Client{Timeout: 10 * time.Minute}}
	})
}

func (p *anthropicProvider) Name() string { return p.name }

// Stream sends the request. A 400 about a reasoning block's signature (a history the API will not take
// back: an edited prefix on an account the drop does not reach, another account's blocks) is recovered
// once as the API documents: the same request without any reasoning block.
func (p *anthropicProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	res, err := p.send(ctx, req, emit)
	if err != nil && hasThinking(req.Messages) && rejectedSignature(err) {
		req.Messages = withoutThinking(req.Messages)
		return p.send(ctx, req, emit)
	}
	return res, err
}

func rejectedSignature(err error) bool {
	m := err.Error()
	return strings.HasPrefix(m, "API 400") && strings.Contains(m, "signature") && strings.Contains(m, "thinking")
}

func (p *anthropicProvider) send(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	headers, sign, err := p.access.credentials(ctx)
	if err != nil {
		return ModelResult{}, err
	}
	body, betas := anthropicBody(req)
	if len(betas) > 0 {
		headers["anthropic-beta"] = strings.Join(betas, ",")
	}
	p.access.adapt(body)
	resp, err := postSigned(ctx, p.client, p.access.url(req.Model), headers, body, sign)
	if err != nil {
		return ModelResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ModelResult{}, apiError(resp)
	}
	return readAnthropicStream(resp.Body, emit)
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
