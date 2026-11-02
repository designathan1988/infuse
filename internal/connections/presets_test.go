package connections

import (
	"context"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/llm"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// presetRequest is what each OpenAI-compatible endpoint accepts, from its published request schema
// (read 2026-11-02; xAI's chat reference could not be read and keeps the OpenAI shape).
var presetRequest = map[string]struct {
	tokens      string
	streamUsage bool
}{
	"openai":     {"max_completion_tokens", true},
	"deepseek":   {"max_tokens", true},
	"openrouter": {"max_completion_tokens", false},
	"groq":       {"max_completion_tokens", true},
	"together":   {"max_tokens", false},
	"mistral":    {"max_tokens", false},
	"xai":        {"max_completion_tokens", true},
	"lmstudio":   {"max_tokens", false},
	"custom":     {"max_tokens", true},
}

// strictEndpoint answers 422 to a field outside what the preset accepts, as the strict vendors do.
func strictEndpoint(tokens string, streamUsage bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, usage := body["stream_options"]
		other := "max_tokens"
		if tokens == other {
			other = "max_completion_tokens"
		}
		if _, bad := body[other]; bad || body[tokens] == nil || usage != streamUsage {
			w.WriteHeader(422)
			_, _ = io.WriteString(w, `{"error":{"message":"Extra inputs are not permitted"}}`)
			return
		}
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
	}))
}

func TestOpenAIPresetsSendOnlyWhatTheyAccept(t *testing.T) {
	testHome(t)
	seen := 0
	for _, preset := range config.ProviderPresets {
		if preset.Kind != "openai" {
			continue
		}
		seen++
		want, ok := presetRequest[preset.ID]
		if !ok {
			t.Fatalf("preset %s: state here what its endpoint accepts", preset.ID)
		}
		srv := strictEndpoint(want.tokens, want.streamUsage)
		s := config.DefaultSettings()
		s.Connections[preset.ID] = config.Connection{Kind: preset.Kind, BaseURL: srv.URL}
		p, err := ProviderFor(s, preset.ID)
		if err != nil {
			t.Fatal(err)
		}
		msgs := []core.Message{{Role: "user", Content: []core.Block{{Type: "text", Text: "hi"}}}}
		res, err := p.Stream(context.Background(), llm.ModelRequest{Model: "m", MaxTokens: 100, Messages: msgs}, func(llm.StreamEvent) {})
		srv.Close()
		if err != nil || res.Usage.Output != 1 {
			t.Fatalf("%s: %v %+v", preset.ID, err, res.Usage)
		}
	}
	if seen != len(presetRequest) {
		t.Fatalf("%d presets, %d stated", seen, len(presetRequest))
	}
}

func TestPreferredFastModelIsNotTheMainOne(t *testing.T) {
	models := []string{"claude-haiku-4-5", "claude-haiku-5-5", "claude-opus-4-8", "claude-sonnet-4-0", "claude-sonnet-5-5"}
	if m := PreferredModel(models); m != "claude-sonnet-5-5" {
		t.Fatalf("main %s", m)
	}
	if m := PreferredFastModel(models, "claude-sonnet-5-5"); m != "claude-haiku-5-5" {
		t.Fatalf("fast %s", m)
	}
	if m := PreferredFastModel([]string{"llama3"}, "llama3"); m != "llama3" {
		t.Fatalf("no fast model: %s", m)
	}
}

func TestConnectingAnthropicKeepsAFastModel(t *testing.T) {
	testHome(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"claude-haiku-5-5"},{"id":"claude-sonnet-5"}]}`)
	}))
	defer srv.Close()
	s := config.DefaultSettings()
	s.Model = "gone"
	_ = config.SaveSettings(s)
	if _, err := ConnectProvider(context.Background(), "anthropic", "sk-test-1234", srv.URL); err != nil {
		t.Fatal(err)
	}
	if s := config.LoadSettings(); s.Model == s.FastModel || s.FastModel != "claude-haiku-5-5" {
		t.Fatalf("model %s, fast %s", s.Model, s.FastModel)
	}
}

func TestCloudPresetsConnect(t *testing.T) {
	testHome(t)
	c, err := ConnectProvider(context.Background(), "bedrock", "AKIDTEST:secret", "")
	if err != nil || !c.Connected || len(c.Models) == 0 {
		t.Fatalf("%+v %v", c, err)
	}
	kind, cfg := ConnectionConfig(config.LoadSettings(), "bedrock")
	if kind != "bedrock" || !strings.Contains(cfg.BaseURL, "bedrock-mantle") || cfg.APIKey != "AKIDTEST:secret" {
		t.Fatalf("%s %+v", kind, cfg)
	}
	if _, err := ConnectProvider(context.Background(), "vertex", "", ""); err == nil {
		t.Fatal("vertex without its project endpoint accepted")
	}
}
