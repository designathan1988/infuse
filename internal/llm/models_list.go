package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Model providers: the wire protocol of each service.
// Model discovery per provider.

func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	get := func(url string, headers map[string]string) ([]byte, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 300 {
			return nil, httpError(name, resp)
		}
		return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	}
	var names []string
	switch name {
	case "anthropic":
		b, err := get(orDefault(cfg.BaseURL, "https://api.anthropic.com")+"/v1/models?limit=100", map[string]string{"x-api-key": cfg.APIKey, "anthropic-version": "2023-06-01"})
		if err != nil {
			return nil, err
		}
		var r struct {
			Data []struct{ ID string } `json:"data"`
		}
		_ = json.Unmarshal(b, &r)
		for _, d := range r.Data {
			names = append(names, d.ID)
		}
	case "openai":
		h := map[string]string{}
		if cfg.APIKey != "" {
			h["authorization"] = "Bearer " + cfg.APIKey
		}
		b, err := get(orDefault(cfg.BaseURL, openAIDefaultBase)+"/v1/models", h)
		if err != nil {
			return nil, err
		}
		var r struct {
			Data []struct{ ID string } `json:"data"`
		}
		_ = json.Unmarshal(b, &r)
		for _, d := range r.Data {
			names = append(names, d.ID)
		}
	case "google":
		b, err := get(orDefault(cfg.BaseURL, googleDefaultBase)+"/v1beta/models?pageSize=200", map[string]string{"x-goog-api-key": cfg.APIKey})
		if err != nil {
			return nil, err
		}
		var r struct {
			Models []struct {
				Name    string   `json:"name"`
				Methods []string `json:"supportedGenerationMethods"`
			} `json:"models"`
		}
		_ = json.Unmarshal(b, &r)
		for _, m := range r.Models {
			for _, me := range m.Methods {
				if me == "generateContent" {
					names = append(names, strings.TrimPrefix(m.Name, "models/"))
					break
				}
			}
		}
	case "ollama":
		b, err := get(orDefault(cfg.BaseURL, ollamaDefaultBase)+"/api/tags", nil)
		if err != nil {
			return nil, fmt.Errorf("Ollama não respondeu (está rodando?): %w", err)
		}
		var r struct {
			Models []struct{ Name string } `json:"models"`
		}
		_ = json.Unmarshal(b, &r)
		for _, m := range r.Models {
			names = append(names, m.Name)
		}
	case "mock":
		names = []string{"mock"}
	default:
		return nil, fmt.Errorf("provedor desconhecido: %s", name)
	}
	sort.Strings(names)
	return names, nil
}
