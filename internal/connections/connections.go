package connections

// Connections: the model providers the user has connected. Each one has a kind (the wire protocol:
// anthropic | openai | google | ollama | mock), an endpoint and a key. Keys live in the OS secret store
// (secrets_*.go), never in settings.json. Sessions pick a connection and a model from it.

import (
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/llm"
	"context"
	"errors"
	"sort"
	"strings"
	"time"
)

type Settings = config.Settings
type Connection = config.Connection
type ProviderConfig = llm.ProviderConfig
type Provider = llm.Provider

var providerPresets = config.ProviderPresets

func presetFor(id string) config.ProviderPreset { return config.PresetFor(id) }
func secretGet(name string) (string, error)      { return config.SecretGet(name) }
func secretSet(name, v string) error             { return config.SecretSet(name, v) }
func secretDelete(name string) error             { return config.SecretDelete(name) }
func secretName(id string) string                { return config.SecretName(id) }
func keyHint(k string) string                    { return config.KeyHint(k) }
func contains(list []string, v string) bool      { return core.Contains(list, v) }
func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	return llm.ListModels(ctx, name, cfg)
}
func newProvider(name string, cfg ProviderConfig) (Provider, error) { return llm.NewProvider(name, cfg) }
func LoadSettings() Settings                    { return config.LoadSettings() }
func SaveSettings(s Settings) error             { return config.SaveSettings(s) }

// connectionConfig resolves a connection id to the provider kind and its config (key from the secret store).
func ConnectionConfig(s Settings, id string) (string, ProviderConfig) {
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

func ProviderFor(s Settings, id string) (Provider, error) {
	kind, cfg := ConnectionConfig(s, id)
	return newProvider(kind, cfg)
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
	base = c.BaseURL
	if base == "" {
		base = p.BaseURL
	}
	cfg := ProviderConfig{APIKey: key, BaseURL: strings.TrimRight(base, "/")}
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
			s.Model = PreferredModel(c.Models)
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


// preferredModel picks a sensible default from a provider's list: a coding/chat model, not an embedding one.
func PreferredModel(models []string) string {
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

type ModelChoice struct {
	Provider string `json:"provider"`
	Label    string `json:"label"`
	Model    string `json:"model"`
}

// AvailableModels lists every model of every connected provider, for the model picker.
func AvailableModels(s Settings) []ModelChoice {
	var out []ModelChoice
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
			out = append(out, ModelChoice{Provider: id, Label: presetFor(id).Label, Model: m})
		}
	}
	if out == nil {
		out = []ModelChoice{}
	}
	return out
}
