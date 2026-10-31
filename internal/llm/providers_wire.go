package llm

// OpenAI (Chat Completions, SSE, function calling; any compatible endpoint via BaseURL: LM Studio, vLLM,
// OpenRouter, Groq…), Google Gemini (streamGenerateContent?alt=sse, functionDeclarations) and Ollama
// (/api/chat NDJSON, no key). Each is registered by name; the loop only knows the Provider interface.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	RegisterProvider("openai", func(cfg ProviderConfig) Provider {
		return &openAIProvider{key: cfg.APIKey, base: orDefault(cfg.BaseURL, openAIDefaultBase), client: httpClient()}
	})
	RegisterProvider("google", func(cfg ProviderConfig) Provider {
		return &googleProvider{key: cfg.APIKey, base: orDefault(cfg.BaseURL, googleDefaultBase), client: httpClient()}
	})
	RegisterProvider("ollama", func(cfg ProviderConfig) Provider {
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
