package llm

// The Anthropic adapter against a fake server emitting the Messages API's exact wire format.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeMessages records every request and answers each with the next script.
type fakeMessages struct {
	mu      sync.Mutex
	bodies  []string
	headers []http.Header
	paths   []string
	replies []func(w http.ResponseWriter)
}

func (f *fakeMessages) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies = append(f.bodies, string(b))
	f.headers = append(f.headers, r.Header.Clone())
	f.paths = append(f.paths, r.URL.Path)
	reply := f.replies[0]
	if len(f.replies) > 1 {
		f.replies = f.replies[1:]
	}
	reply(w)
}

func sse(lines ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		w.Header().Set("content-type", "text/event-stream")
		for _, l := range lines {
			name := l[:strings.Index(l, " ")]
			_, _ = io.WriteString(w, "event: "+name+"\ndata: "+l[len(name)+1:]+"\n\n")
		}
	}
}

func status(code int, body string, headers ...string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) {
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}
}

var fullReply = sse(
	`message_start {"type":"message_start","message":{"id":"msg_1","role":"assistant","usage":{"input_tokens":11,"cache_read_input_tokens":100,"cache_creation_input_tokens":20,"output_tokens":1}}}`,
	`content_block_start {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`,
	`content_block_delta {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me "}}`,
	`content_block_delta {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"check."}}`,
	`content_block_delta {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-A"}}`,
	`content_block_stop {"type":"content_block_stop","index":0}`,
	`content_block_start {"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque"}}`,
	`content_block_stop {"type":"content_block_stop","index":1}`,
	`content_block_start {"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
	`content_block_delta {"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Reading."}}`,
	`content_block_stop {"type":"content_block_stop","index":2}`,
	`content_block_start {"type":"content_block_start","index":3,"content_block":{"type":"tool_use","id":"tu_1","name":"Read","input":{}}}`,
	`content_block_delta {"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"{\"pa"}}`,
	`content_block_delta {"type":"content_block_delta","index":3,"delta":{"type":"input_json_delta","partial_json":"th\":\"a\"}"}}`,
	`content_block_stop {"type":"content_block_stop","index":3}`,
	`message_delta {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":42}}`,
	`message_stop {"type":"message_stop"}`,
)

func anthropicAt(t *testing.T, f *fakeMessages) Provider {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	p, err := newProvider("anthropic", ProviderConfig{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal([]byte(got), &g); err != nil {
		t.Fatalf("got: %v: %s", err, got)
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("want: %v: %s", err, want)
	}
	if !reflect.DeepEqual(g, w) {
		t.Fatalf("request differs\n got: %s\nwant: %s", got, want)
	}
}

func TestAnthropicStreamExactRequest(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){fullReply}}
	p := anthropicAt(t, f)
	emit, evs := collect()
	res, err := p.Stream(context.Background(), ModelRequest{Model: "claude-sonnet-5-5", System: "sys", Messages: convo[:1], Tools: toolSpecs(), MaxTokens: 16000, Thinking: 4000, Effort: "medium"}, emit)
	if err != nil {
		t.Fatal(err)
	}
	sameJSON(t, f.bodies[0], `{"model":"claude-sonnet-5-5","max_tokens":16000,"stream":true,
		"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}],
		"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}],
		"tools":[{"name":"Read","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],
		"thinking":{"type":"adaptive","display":"summarized"},
		"output_config":{"effort":"medium"}}`)
	h := f.headers[0]
	if f.paths[0] != "/v1/messages" || h.Get("x-api-key") != "k" || h.Get("anthropic-version") != "2023-06-01" || h.Get("anthropic-beta") != "" {
		t.Fatalf("%s %v", f.paths[0], h)
	}
	want := []Block{
		{Type: "thinking", Thinking: "Let me check.", Signature: "sig-A"},
		{Type: "redacted_thinking", Data: "opaque"},
		{Type: "text", Text: "Reading."},
		{Type: "tool_use", ID: "tu_1", Name: "Read", Input: json.RawMessage(`{"path":"a"}`)},
	}
	if !reflect.DeepEqual(res.Content, want) {
		t.Fatalf("blocks: %+v", res.Content)
	}
	if res.StopReason != "tool_use" || res.Usage != (Usage{Input: 11, Output: 42, CacheRead: 100, CacheWrite: 20}) {
		t.Fatalf("%s %+v", res.StopReason, res.Usage)
	}
	if streamedText(*evs) != "Reading." {
		t.Fatalf("streamed %q", streamedText(*evs))
	}
}

// Reasoning goes back verbatim, an empty text included; what has no signature never goes.
func TestAnthropicReplaysThinkingVerbatim(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){fullReply}}
	p := anthropicAt(t, f)
	msgs := []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "hi"}}},
		{Role: "assistant", Content: []Block{
			{Type: "thinking", Thinking: "", Signature: "sig-empty"},
			{Type: "thinking", Thinking: "from deepseek"},
			{Type: "redacted_thinking", Data: "opaque"},
			{Type: "tool_use", ID: "c1", Name: "Read", Input: json.RawMessage(`{"path":"a"}`)},
		}},
		{Role: "user", Content: []Block{{Type: "tool_result", ToolUseID: "c1", Content: "x"}}},
	}
	if _, err := p.Stream(context.Background(), ModelRequest{Model: "claude-opus-5-5", Messages: msgs, MaxTokens: 16000, Thinking: 4000, Effort: "high"}, func(StreamEvent) {}); err != nil {
		t.Fatal(err)
	}
	sameJSON(t, f.bodies[0], `{"model":"claude-opus-5-5","max_tokens":16000,"stream":true,
		"thinking":{"type":"adaptive","display":"summarized","block_binding":{"prefix_mismatch_behavior":"drop_block"}},
		"output_config":{"effort":"high"},
		"messages":[{"role":"user","content":[{"type":"text","text":"hi"}]},
		{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"sig-empty"},{"type":"redacted_thinking","data":"opaque"},{"type":"tool_use","id":"c1","name":"Read","input":{"path":"a"}}]},
		{"role":"user","content":[{"type":"tool_result","tool_use_id":"c1","content":"x","is_error":false}]}]}`)
	if f.headers[0].Get("anthropic-beta") != "thinking-binding-controls-2026-08-01" {
		t.Fatalf("beta header: %v", f.headers[0])
	}
}

// A 400 on a signature is recovered once, without any reasoning block in the history.
func TestAnthropicRejectedSignatureRetriesWithoutThinking(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){
		status(400, `{"type":"error","error":{"type":"invalid_request_error","message":"messages.1.content.0: Invalid `+"`signature`"+` in `+"`thinking`"+` block."}}`),
		fullReply,
	}}
	p := anthropicAt(t, f)
	msgs := []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "hi"}}},
		{Role: "assistant", Content: []Block{{Type: "thinking", Thinking: "x", Signature: "stale"}, {Type: "text", Text: "ok"}}},
		{Role: "user", Content: []Block{{Type: "text", Text: "go on"}}},
	}
	if _, err := p.Stream(context.Background(), ModelRequest{Model: "claude-opus-5", Messages: msgs, MaxTokens: 16000, Thinking: 4000}, func(StreamEvent) {}); err != nil {
		t.Fatal(err)
	}
	if len(f.bodies) != 2 || !strings.Contains(f.bodies[0], "stale") || strings.Contains(f.bodies[1], "stale") {
		t.Fatalf("%d requests: %v", len(f.bodies), f.bodies)
	}
}

func TestAnthropicErrorMapping(t *testing.T) {
	cases := []struct {
		name  string
		reply func(http.ResponseWriter)
		want  string
	}{
		{"auth", status(401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`), "API 401: invalid x-api-key"},
		{"not json", status(400, `plain failure`), "API 400: plain failure"},
		{"stream error", sse(`message_start {"message":{"usage":{"input_tokens":1}}}`, `error {"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`), "stream: Overloaded"},
		{"cut stream", sse(`message_start {"message":{"usage":{"input_tokens":1}}}`), "the stream ended before the message did"},
		{"refusal", sse(`message_start {"message":{"usage":{"input_tokens":1}}}`, `message_delta {"delta":{"stop_reason":"refusal","stop_details":{"type":"refusal","category":"cyber"}},"usage":{"output_tokens":1}}`), "the model declined this request (cyber)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := anthropicAt(t, &fakeMessages{replies: []func(http.ResponseWriter){c.reply}})
			_, err := p.Stream(context.Background(), ModelRequest{Model: "claude-opus-5-5", Messages: convo[:1], MaxTokens: 100}, func(StreamEvent) {})
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestAnthropicRetriesRateLimitWithRetryAfter(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){
		status(429, `{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, "Retry-After", "1"),
		status(529, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`, "Retry-After", "1"),
		fullReply,
	}}
	res, err := anthropicAt(t, f).Stream(context.Background(), ModelRequest{Model: "claude-opus-5-5", Messages: convo[:1], MaxTokens: 100}, func(StreamEvent) {})
	if err != nil || len(f.bodies) != 3 || res.Usage.Output != 42 {
		t.Fatalf("%v, %d requests, %+v", err, len(f.bodies), res.Usage)
	}
	if f.bodies[0] != f.bodies[2] {
		t.Fatal("a retry must send the same request")
	}
}

func TestAnthropicMissingKey(t *testing.T) {
	p, _ := newProvider("anthropic", ProviderConfig{})
	if _, err := p.Stream(context.Background(), ModelRequest{Model: "m"}, func(StreamEvent) {}); err == nil || !strings.Contains(err.Error(), "key") {
		t.Fatalf("err = %v", err)
	}
}

// What each family gets, from the table: the request that used to answer 400 must not be sent.
func TestAnthropicRequestPerFamily(t *testing.T) {
	cases := []struct {
		model, effort string
		budget        int
		thinking      string // the thinking field; "" = absent
		effort2       string // output_config.effort; "" = absent
	}{
		{"claude-opus-5-5", "medium", 4000, `{"type":"adaptive","display":"summarized"}`, "medium"},
		{"claude-opus-5-5", "low", 0, "", "low"},
		{"claude-opus-5-5", "", 0, "", "low"},
		{"anthropic.claude-opus-5-5", "high", 12000, `{"type":"adaptive","display":"summarized"}`, "high"},
		{"claude-sonnet-5-5", "low", 0, `{"type":"between_tools"}`, "low"},
		{"claude-sonnet-5-5", "max", 32000, `{"type":"adaptive","display":"summarized"}`, "max"},
		{"claude-sonnet-5", "low", 0, `{"type":"disabled"}`, "low"},
		{"claude-haiku-5-5", "", 0, `{"type":"disabled"}`, ""},
		{"claude-haiku-5-5", "high", 12000, `{"type":"adaptive","display":"summarized"}`, "high"},
		{"claude-fable-5-1", "medium", 4000, `{"type":"adaptive","display":"summarized"}`, "medium"},
		{"claude-fable-5-1", "", 0, "", "low"},
		{"claude-opus-5", "high", 12000, `{"type":"adaptive","display":"summarized"}`, "high"},
		{"claude-opus-4-8", "", 0, "", ""},
		{"claude-opus-4-7", "max", 32000, `{"type":"adaptive","display":"summarized"}`, "max"},
		{"claude-opus-4-6", "high", 12000, `{"type":"adaptive"}`, "high"},
		{"claude-opus-4-5-20251101", "max", 32000, `{"type":"enabled","budget_tokens":32000}`, "high"},
		{"claude-haiku-4-5-20251001", "medium", 4000, `{"type":"enabled","budget_tokens":4000}`, ""},
		{"claude-sonnet-4-20250514", "high", 500, `{"type":"enabled","budget_tokens":1024}`, ""},
		{"claude-sonnet-9", "high", 12000, `{"type":"adaptive","display":"summarized"}`, "high"},
		{"glm-5", "high", 12000, `{"type":"enabled","budget_tokens":12000}`, ""},
	}
	for _, c := range cases {
		body, _ := anthropicBody(ModelRequest{Model: c.model, Effort: c.effort, Thinking: c.budget, MaxTokens: 64000})
		got := ""
		if v, ok := body["thinking"]; ok {
			b, _ := json.Marshal(v)
			got = string(b)
		}
		if c.thinking == "" && got != "" || c.thinking != "" && got == "" {
			t.Fatalf("%s: thinking %s, want %s", c.model, got, c.thinking)
		}
		if got != "" {
			sameJSON(t, got, c.thinking)
		}
		cfg, _ := body["output_config"].(map[string]any)
		if e, _ := cfg["effort"].(string); e != c.effort2 {
			t.Fatalf("%s/%s: effort %q, want %q", c.model, c.effort, e, c.effort2)
		}
	}
}

func TestAlwaysThinkingModelsKeepRoomForTheAnswer(t *testing.T) {
	short := ModelRequest{Model: "claude-opus-5-5", MaxTokens: 60}
	if body, _ := anthropicBody(short); body["max_tokens"] != thinkingFloor {
		t.Fatalf("max_tokens %v", body["max_tokens"])
	}
	short.Model = "claude-haiku-5-5" // reasoning is off here: the caller's limit stands
	if body, _ := anthropicBody(short); body["max_tokens"] != 60 {
		t.Fatalf("max_tokens %v", body["max_tokens"])
	}
}

// Turning reasoning off clears the history of it: between_tools takes no other field.
func TestThinkingOffDropsTheHistoryOfIt(t *testing.T) {
	msgs := []Message{{Role: "assistant", Content: []Block{{Type: "thinking", Thinking: "t", Signature: "s"}, {Type: "text", Text: "ok"}}}}
	body, betas := anthropicBody(ModelRequest{Model: "claude-sonnet-5-5", Messages: msgs, MaxTokens: 100})
	b, _ := json.Marshal(body)
	sameJSON(t, string(b), `{"model":"claude-sonnet-5-5","max_tokens":100,"stream":true,"thinking":{"type":"between_tools"},"messages":[{"role":"assistant","content":[{"type":"text","text":"ok"}]}]}`)
	if betas != nil {
		t.Fatalf("betas %v", betas)
	}
}

func TestForcedToolPerFamily(t *testing.T) {
	spec := ToolSpec{Name: "Facts", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{"facts":{"type":"array","items":{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}}},"required":["facts"]}`)}
	req := ModelRequest{System: "sys", Tools: []ToolSpec{spec}, ForceTool: "Facts", MaxTokens: 4000}
	for _, model := range []string{"claude-opus-5", "claude-haiku-5-5", "claude-fable-5", "claude-opus-4-8"} {
		req.Model = model
		body, _ := anthropicBody(req)
		b, _ := json.Marshal(body["tool_choice"])
		sameJSON(t, string(b), `{"type":"tool","name":"Facts"}`)
	}
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-fable-5-1", "claude-mythos-5-1", "anthropic.claude-opus-5-5"} {
		req.Model = model
		body, _ := anthropicBody(req)
		choice, _ := json.Marshal(body["tool_choice"])
		sameJSON(t, string(choice), `{"type":"auto","disable_parallel_tool_use":true}`)
		tools, _ := json.Marshal(body["tools"])
		sameJSON(t, string(tools), `[{"name":"Facts","description":"d","strict":true,"input_schema":{"type":"object","additionalProperties":false,"required":["facts"],
			"properties":{"facts":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["q"],"properties":{"q":{"type":"string"}}}}}}}]`)
		sys, _ := json.Marshal(body["system"])
		if !strings.Contains(string(sys), "calling the tool Facts exactly once") {
			t.Fatalf("%s: system %s", model, sys)
		}
	}
}

// askJSON end to end on the default fast path when it equals the main model: the model answers in
// text first, gets the correction, and calls the tool.
func TestAskJSONWithoutForcedCall(t *testing.T) {
	f := &fakeMessages{replies: []func(http.ResponseWriter){
		sse(`message_start {"message":{"usage":{"input_tokens":5}}}`,
			`content_block_start {"index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`content_block_delta {"index":0,"delta":{"type":"signature_delta","signature":"s1"}}`,
			`content_block_stop {"index":0}`,
			`content_block_start {"index":1,"content_block":{"type":"text","text":""}}`,
			`content_block_delta {"index":1,"delta":{"type":"text_delta","text":"I cannot say"}}`,
			`content_block_stop {"index":1}`,
			`message_delta {"delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`),
		sse(`message_start {"message":{"usage":{"input_tokens":7}}}`,
			`content_block_start {"index":0,"content_block":{"type":"tool_use","id":"t1","name":"Facts"}}`,
			`content_block_delta {"index":0,"delta":{"type":"input_json_delta","partial_json":"{\"facts\":[\"x\"]}"}}`,
			`content_block_stop {"index":0}`,
			`message_delta {"delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":4}}`),
	}}
	var out struct {
		Facts []string `json:"facts"`
	}
	spec := ToolSpec{Name: "Facts", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{"facts":{"type":"array","items":{"type":"string"}}},"required":["facts"]}`)}
	u, err := AskJSON(context.Background(), anthropicAt(t, f), "claude-opus-5-5", "sys", "text", spec, 4000, &out)
	if err != nil || len(out.Facts) != 1 || u.Input != 12 || u.Output != 7 {
		t.Fatalf("%v %+v %+v", err, out, u)
	}
	if strings.Contains(f.bodies[0], `"type":"tool"`) || !strings.Contains(f.bodies[1], `"signature":"s1"`) {
		t.Fatalf("bodies: %v", f.bodies)
	}
}

func TestFamilyOfLongestPrefix(t *testing.T) {
	for model, wantPrefix := range map[string]string{
		"claude-opus-5-5":                   "claude-opus-5-5",
		"claude-opus-5":                     "claude-opus-5",
		"claude-fable-5-1":                  "claude-fable-5-1",
		"claude-fable-5":                    "claude-fable-5",
		"us.anthropic.claude-sonnet-5-5":    "claude-sonnet-5-5",
		"claude-opus-4-5@20251101":          "claude-opus-4-5",
		"claude-haiku-4-5-20251001":         "claude-haiku-4-5",
		"anthropic.claude-haiku-5-5":        "claude-haiku-5-5",
		"anthropic.claude-sonnet-4-5-v1:0":  "claude-sonnet-4-5",
		"global.anthropic.claude-opus-4-6-": "claude-opus-4-6",
	} {
		if got := familyOf(model).prefix; got != wantPrefix {
			t.Fatalf("%s: %q", model, got)
		}
	}
}
