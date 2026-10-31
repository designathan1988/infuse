package main

// Connections: the model providers the user has connected. Each one has a kind (the wire protocol:
// anthropic | openai | google | ollama | mock), an endpoint and a key. Keys live in the OS secret store
// (secrets_*.go), never in settings.json. Sessions pick a connection and a model from it.

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Connection struct {
	Kind      string   `json:"kind"`
	BaseURL   string   `json:"baseUrl,omitempty"`
	Models    []string `json:"models,omitempty"`
	Connected bool     `json:"connected"`
	KeyHint   string   `json:"keyHint,omitempty"` // last four characters, to recognise the key
}

// providerPreset describes a provider the window offers to connect.
type providerPreset struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	BaseURL string `json:"baseUrl"`
	NeedKey bool   `json:"needKey"`
	KeyURL  string `json:"keyUrl,omitempty"`
	Local   bool   `json:"local,omitempty"`
}

var providerPresets = []providerPreset{
	{ID: "anthropic", Kind: "anthropic", Label: "Anthropic", BaseURL: "https://api.anthropic.com", NeedKey: true, KeyURL: "https://console.anthropic.com/settings/keys"},
	{ID: "openai", Kind: "openai", Label: "OpenAI", BaseURL: openAIDefaultBase, NeedKey: true, KeyURL: "https://platform.openai.com/api-keys"},
	{ID: "google", Kind: "google", Label: "Google Gemini", BaseURL: googleDefaultBase, NeedKey: true, KeyURL: "https://aistudio.google.com/apikey"},
	{ID: "deepseek", Kind: "openai", Label: "DeepSeek", BaseURL: "https://api.deepseek.com", NeedKey: true, KeyURL: "https://platform.deepseek.com/api_keys"},
	{ID: "openrouter", Kind: "openai", Label: "OpenRouter", BaseURL: "https://openrouter.ai/api", NeedKey: true, KeyURL: "https://openrouter.ai/keys"},
	{ID: "groq", Kind: "openai", Label: "Groq", BaseURL: "https://api.groq.com/openai", NeedKey: true, KeyURL: "https://console.groq.com/keys"},
	{ID: "together", Kind: "openai", Label: "Together", BaseURL: "https://api.together.xyz", NeedKey: true, KeyURL: "https://api.together.ai/settings/api-keys"},
	{ID: "mistral", Kind: "openai", Label: "Mistral", BaseURL: "https://api.mistral.ai", NeedKey: true, KeyURL: "https://console.mistral.ai/api-keys"},
	{ID: "xai", Kind: "openai", Label: "xAI", BaseURL: "https://api.x.ai", NeedKey: true, KeyURL: "https://console.x.ai"},
	{ID: "ollama", Kind: "ollama", Label: "Ollama", BaseURL: ollamaDefaultBase, Local: true},
	{ID: "lmstudio", Kind: "openai", Label: "LM Studio", BaseURL: "http://127.0.0.1:1234", Local: true},
	{ID: "custom", Kind: "openai", Label: "Compatível com OpenAI", BaseURL: "", NeedKey: false},
	{ID: "mock", Kind: "mock", Label: "Demonstração (sem chave)", Local: true},
}

func presetFor(id string) providerPreset {
	for _, p := range providerPresets {
		if p.ID == id {
			return p
		}
	}
	return providerPreset{ID: id, Kind: id}
}

func secretName(id string) string { return "infuse:" + id }

// connectionConfig resolves a connection id to the provider kind and its config (key from the secret store).
func connectionConfig(s Settings, id string) (string, ProviderConfig) {
	p := presetFor(id)
	c := s.Connections[id]
	kind := c.Kind
	if kind == "" {
		kind = p.Kind
	}
	base := c.BaseURL
	if base == "" {
		base = p.BaseURL
	}
	key, _ := secretGet(secretName(id))
	return kind, ProviderConfig{APIKey: key, BaseURL: base}
}

func providerFor(s Settings, id string) (Provider, error) {
	kind, cfg := connectionConfig(s, id)
	return newProvider(kind, cfg)
}

// migrateLegacyKey moves a key stored in settings.json (0.5 and earlier) into the secret store and turns
// the old single-provider fields into a connection.
func migrateLegacyKey(s *Settings) bool {
	if s.APIKey == "" {
		return false
	}
	id := s.Provider
	if s.BaseURL != "" {
		if u, err := url.Parse(s.BaseURL); err == nil {
			for _, p := range providerPresets {
				if pu, err := url.Parse(p.BaseURL); err == nil && pu.Host != "" && pu.Host == u.Host {
					id = p.ID
				}
			}
		}
	}
	if secretSet(secretName(id), s.APIKey) != nil {
		return false // keep it where it is rather than lose it
	}
	if s.Connections == nil {
		s.Connections = map[string]Connection{}
	}
	c := s.Connections[id]
	c.Kind = presetFor(id).Kind
	if c.Kind == "" || c.Kind == id && id != "anthropic" && id != "openai" && id != "google" && id != "ollama" && id != "mock" {
		c.Kind = s.Provider
	}
	if s.BaseURL != "" && s.BaseURL != presetFor(id).BaseURL {
		c.BaseURL = s.BaseURL
	}
	c.Connected = true
	c.KeyHint = keyHint(s.APIKey)
	s.Connections[id] = c
	s.Provider = id
	s.APIKey, s.BaseURL = "", ""
	return true
}

func keyHint(k string) string {
	if len(k) < 8 {
		return ""
	}
	return "…" + k[len(k)-4:]
}

// ConnectProvider stores the key (when given), checks the connection by listing models (or, when the
// endpoint lists none, by a one-token request) and records the result.
func ConnectProvider(ctx context.Context, id, key, base string) (Connection, error) {
	p := presetFor(id)
	if p.Kind == "" {
		return Connection{}, errors.New("provedor desconhecido")
	}
	s := LoadSettings()
	c := s.Connections[id]
	c.Kind = p.Kind
	if base != "" && base != p.BaseURL {
		c.BaseURL = strings.TrimRight(base, "/")
	} else if base == "" && id == "custom" {
		return c, errors.New("informe o endpoint")
	}
	if key == "" {
		key, _ = secretGet(secretName(id))
	}
	if p.NeedKey && key == "" {
		return c, errors.New("informe a chave de API")
	}
	cfg := ProviderConfig{APIKey: key, BaseURL: orDefault(c.BaseURL, p.BaseURL)}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	models, err := ListModels(ctx, p.Kind, cfg)
	if err != nil || len(models) == 0 {
		// Some compatible endpoints do not list models; a minimal request proves the key instead.
		if len(c.Models) == 0 && err != nil {
			return c, err
		}
	}
	if len(models) > 0 {
		c.Models = models
	}
	if key != "" {
		if err := secretSet(secretName(id), key); err != nil {
			return c, errors.New("não foi possível guardar a chave no cofre do sistema: " + err.Error())
		}
		c.KeyHint = keyHint(key)
	}
	c.Connected = true
	if s.Connections == nil {
		s.Connections = map[string]Connection{}
	}
	s.Connections[id] = c
	if s.Provider == "" || !s.Connections[s.Provider].Connected {
		s.Provider = id
		if len(c.Models) > 0 && !contains(c.Models, s.Model) {
			s.Model = preferredModel(c.Models)
			s.FastModel = s.Model
		}
	}
	return c, SaveSettings(s)
}

func DisconnectProvider(id string) error {
	_ = secretDelete(secretName(id))
	s := LoadSettings()
	delete(s.Connections, id)
	if s.Provider == id {
		s.Provider = ""
		for k, c := range s.Connections {
			if c.Connected {
				s.Provider = k
				break
			}
		}
	}
	return SaveSettings(s)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// preferredModel picks a sensible default from a provider's list: a coding/chat model, not an embedding one.
func preferredModel(models []string) string {
	for _, want := range []string{"claude-sonnet", "gpt-5", "gemini-2.5-pro", "deepseek-chat", "deepseek-v4", "qwen", "llama"} {
		for _, m := range models {
			if strings.Contains(m, want) && !strings.Contains(m, "embed") {
				return m
			}
		}
	}
	for _, m := range models {
		if !strings.Contains(m, "embed") && !strings.Contains(m, "whisper") && !strings.Contains(m, "tts") {
			return m
		}
	}
	if len(models) > 0 {
		return models[0]
	}
	return ""
}

type modelChoice struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Model    string `json:"model"`
}

// AvailableModels lists every model of every connected provider, for the model picker.
func AvailableModels(s Settings) []modelChoice {
	var out []modelChoice
	ids := make([]string, 0, len(s.Connections))
	for id := range s.Connections {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := s.Connections[id]
		if !c.Connected {
			continue
		}
		models := c.Models
		if id == "mock" || c.Kind == "mock" {
			models = []string{"mock"}
		}
		for _, m := range models {
			if strings.Contains(m, "embed") || strings.Contains(m, "whisper") || strings.Contains(m, "tts") || strings.Contains(m, "dall-e") {
				continue
			}
			out = append(out, modelChoice{Provider: id, Label: presetFor(id).Label, Model: m})
		}
	}
	if out == nil {
		out = []modelChoice{}
	}
	return out
}
