package config

// Connections: the model providers the user has connected. Each one has a kind (the wire protocol:
// anthropic | bedrock | vertex | openai | google | ollama | mock), an endpoint and a key. Keys live in the OS secret store,
// never in settings.json. The preset table is data (internal/data/providers.json).

import (
	"encoding/json"
	"infuse/internal/data"
	"net/url"
)

type Connection struct {
	Kind      string   `json:"kind"`
	BaseURL   string   `json:"baseUrl,omitempty"`
	Models    []string `json:"models,omitempty"`
	Connected bool     `json:"connected"`
	KeyHint   string   `json:"keyHint,omitempty"` // last four characters, to recognise the key
}

// ProviderPreset describes a provider the window offers to connect.
type ProviderPreset struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	BaseURL string `json:"baseUrl"`
	NeedKey bool   `json:"needKey"`
	KeyURL  string `json:"keyUrl,omitempty"`
	Local   bool   `json:"local,omitempty"`
	// What an OpenAI-compatible endpoint accepts, where it differs from OpenAI.
	MaxTokensField string `json:"maxTokensField,omitempty"` // default max_completion_tokens
	NoStreamUsage  bool   `json:"noStreamUsage,omitempty"`  // stream_options is rejected or has no effect
}

func loadProviderPresets() []ProviderPreset {
	var out []ProviderPreset
	if err := json.Unmarshal(data.Read("providers.json"), &out); err != nil {
		panic("infuse: providers.json is invalid: " + err.Error())
	}
	return out
}

var ProviderPresets = loadProviderPresets()

func PresetFor(id string) ProviderPreset {
	for _, p := range ProviderPresets {
		if p.ID == id {
			return p
		}
	}
	return ProviderPreset{ID: id, Kind: id}
}

func SecretName(id string) string { return "infuse:" + id }

func KeyHint(k string) string {
	if len(k) < 8 {
		return ""
	}
	return "…" + k[len(k)-4:]
}

// MigrateLegacyKey moves a key stored in settings.json (0.5 and earlier) into the secret store and turns
// the old single-provider fields into a connection.
func MigrateLegacyKey(s *Settings) bool {
	if s.APIKey == "" {
		return false
	}
	id := s.Provider
	if s.BaseURL != "" {
		if u, err := url.Parse(s.BaseURL); err == nil {
			for _, p := range ProviderPresets {
				if pu, err := url.Parse(p.BaseURL); err == nil && pu.Host != "" && pu.Host == u.Host {
					id = p.ID
				}
			}
		}
	}
	if SecretSet(SecretName(id), s.APIKey) != nil {
		return false // keep it where it is rather than lose it
	}
	if s.Connections == nil {
		s.Connections = map[string]Connection{}
	}
	c := s.Connections[id]
	c.Kind = PresetFor(id).Kind
	if c.Kind == "" || c.Kind == id && id != "anthropic" && id != "openai" && id != "google" && id != "ollama" && id != "mock" {
		c.Kind = s.Provider
	}
	if s.BaseURL != "" && s.BaseURL != PresetFor(id).BaseURL {
		c.BaseURL = s.BaseURL
	}
	c.Connected = true
	c.KeyHint = KeyHint(s.APIKey)
	s.Connections[id] = c
	s.Provider = id
	s.APIKey, s.BaseURL = "", ""
	return true
}
