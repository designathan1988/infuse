package llm

// Provider adapters against fake servers emitting each provider's exact wire format: streamed text,
// tool arguments reassembled across chunks, token counts, request shape.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func collect() (func(StreamEvent), *[]StreamEvent) {
	var evs []StreamEvent
	return func(e StreamEvent) { evs = append(evs, e) }, &evs
}

func streamedText(evs []StreamEvent) string {
	var b strings.Builder
	for _, e := range evs {
		if e.Kind == "text" {
			b.WriteString(e.Text)
		}
	}
	return b.String()
}

var convo = []Message{
	{Role: "user", Content: []Block{{Type: "text", Text: "hi"}}},
	{Role: "assistant", Content: []Block{{Type: "text", Text: "I will read"}, {Type: "tool_use", ID: "c1", Name: "Read", Input: json.RawMessage(`{"path":"a"}`)}}},
	{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "c1", Content: "content"}}},
}

func TestOpenAIStreamToolsUsage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("authorization") != "Bearer k" {
			t.Errorf("request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("content-type", "text/event-stream")
		for _, c := range []string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"Hello"}}]}`,
			`data: {"choices":[{"delta":{"content":", world"}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"Write","arguments":"{\"pa"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.go\",\"content\":\"x\"}"}}]}}]}`,
			`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":12,"completion_tokens":7,"prompt_tokens_details":{"cached_tokens":4}}}`,
			`data: [DONE]`,
		} {
			_, _ = io.WriteString(w, c+"\n\n")
		}
	}))
	defer srv.Close()
	p, _ := newProvider("openai", ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	emit, evs := collect()
	res, err := p.Stream(context.Background(), ModelRequest{Model: "gpt-test", System: "sys", Messages: convo, Tools: toolSpecs(), MaxTokens: 100}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if streamedText(*evs) != "Hello, world" {
		t.Fatalf("streamed %q", streamedText(*evs))
	}
	if len(res.Content) != 2 || res.Content[0].Text != "Hello, world" || res.Content[1].Name != "Write" || string(res.Content[1].Input) != `{"path":"a.go","content":"x"}` {
		t.Fatalf("blocks: %+v", res.Content)
	}
	if res.Usage.Input != 8 || res.Usage.Output != 7 || res.Usage.CacheRead != 4 || res.StopReason != "tool_use" {
		t.Fatalf("usage/stop: %+v %s", res.Usage, res.StopReason)
	}
	// request shape: system first, tool result as role:tool, tools as functions
	msgs := gotBody["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || msgs[3].(map[string]any)["role"] != "tool" || msgs[3].(map[string]any)["tool_call_id"] != "c1" {
		t.Fatalf("messages: %v", msgs)
	}
	if gotBody["tools"].([]any)[0].(map[string]any)["type"] != "function" {
		t.Fatal("tools not function-typed")
	}
}

func TestGoogleStreamFunctionCallUsage(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/v1beta/models/gemini-test:streamGenerateContent") || r.URL.Query().Get("alt") != "sse" || r.Header.Get("x-goog-api-key") != "gk" {
			t.Errorf("request: %s %s", r.URL.Path, r.URL.RawQuery)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("content-type", "text/event-stream")
		for _, c := range []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"thinking","thought":true}],"role":"model"}}]}`,
			`data: {"candidates":[{"content":{"parts":[{"text":"I will create "}],"role":"model"}}]}`,
			`data: {"candidates":[{"content":{"parts":[{"text":"the file."}],"role":"model"}}]}`,
			`data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"Write","args":{"path":"b.ts","content":"y"}}}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":9,"candidatesTokenCount":3,"thoughtsTokenCount":2}}`,
		} {
			_, _ = io.WriteString(w, c+"\r\n\r\n")
		}
	}))
	defer srv.Close()
	p, _ := newProvider("google", ProviderConfig{APIKey: "gk", BaseURL: srv.URL})
	emit, evs := collect()
	res, err := p.Stream(context.Background(), ModelRequest{Model: "gemini-test", System: "sys", Messages: convo, Tools: toolSpecs(), Thinking: 1024}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if streamedText(*evs) != "I will create the file." {
		t.Fatalf("streamed %q", streamedText(*evs))
	}
	if len(res.Content) != 3 || res.Content[0].Type != "thinking" || res.Content[1].Text != "I will create the file." || res.Content[2].Name != "Write" || !strings.Contains(string(res.Content[2].Input), `"b.ts"`) {
		t.Fatalf("blocks: %+v", res.Content)
	}
	if res.Usage.Input != 9 || res.Usage.Output != 5 || res.StopReason != "tool_use" {
		t.Fatalf("usage: %+v %s", res.Usage, res.StopReason)
	}
	contents := gotBody["contents"].([]any)
	last := contents[2].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionResponse"].(map[string]any)
	if last["name"] != "Read" {
		t.Fatalf("functionResponse must carry the function name: %v", last)
	}
	if gotBody["systemInstruction"] == nil || gotBody["generationConfig"].(map[string]any)["thinkingConfig"] == nil {
		t.Fatal("systemInstruction / thinkingConfig missing")
	}
	decl := gotBody["tools"].([]any)[0].(map[string]any)["functionDeclarations"].([]any)[0].(map[string]any)
	if _, has := decl["parameters"].(map[string]any)["additionalProperties"]; has {
		t.Fatal("schema keyword Gemini rejects was sent")
	}
}

func TestOllamaNDJSONToolsUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		if body["stream"] != true || body["model"] != "llama-test" {
			t.Errorf("body %v", body)
		}
		w.Header().Set("content-type", "application/x-ndjson")
		for _, l := range []string{
			`{"model":"llama-test","message":{"role":"assistant","content":"writing "},"done":false}`,
			`{"model":"llama-test","message":{"role":"assistant","content":"the file"},"done":false}`,
			`{"model":"llama-test","message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"Bash","arguments":{"command":"ls"}}}]},"done":true,"prompt_eval_count":20,"eval_count":11}`,
		} {
			_, _ = io.WriteString(w, l+"\n")
		}
	}))
	defer srv.Close()
	p, _ := newProvider("ollama", ProviderConfig{BaseURL: srv.URL})
	emit, evs := collect()
	res, err := p.Stream(context.Background(), ModelRequest{Model: "llama-test", Messages: convo, Tools: toolSpecs()}, emit)
	if err != nil {
		t.Fatal(err)
	}
	if streamedText(*evs) != "writing the file" || len(res.Content) != 2 || res.Content[1].Name != "Bash" || string(res.Content[1].Input) != `{"command":"ls"}` {
		t.Fatalf("blocks: %+v", res.Content)
	}
	if res.Usage.Input != 20 || res.Usage.Output != 11 || res.StopReason != "tool_use" {
		t.Fatalf("usage: %+v", res.Usage)
	}
}

func TestOllamaListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"models":[{"name":"qwen2.5-coder:7b"},{"name":"llama3.1:8b"}]}`)
	}))
	defer srv.Close()
	names, err := ListModels(context.Background(), "ollama", ProviderConfig{BaseURL: srv.URL})
	if err != nil || len(names) != 2 || names[0] != "llama3.1:8b" {
		t.Fatalf("%v %v", names, err)
	}
}

func TestOpenAIHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"error":{"message":"Incorrect API key"}}`)
	}))
	defer srv.Close()
	p, _ := newProvider("openai", ProviderConfig{APIKey: "bad", BaseURL: srv.URL})
	_, err := p.Stream(context.Background(), ModelRequest{Model: "m", Messages: convo}, func(StreamEvent) {})
	if err == nil || !strings.Contains(err.Error(), "Incorrect API key") {
		t.Fatalf("err %v", err)
	}
}

func TestProvidersRegistered(t *testing.T) {
	for _, n := range []string{"anthropic", "openai", "google", "ollama", "mock"} {
		if _, err := newProvider(n, ProviderConfig{}); err != nil {
			t.Fatal(n, err)
		}
	}
	if _, err := newProvider("nope", ProviderConfig{}); err == nil {
		t.Fatal("unknown accepted")
	}
}
