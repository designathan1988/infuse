package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/platform/process"
	"io"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Model providers: the wire protocol of each service.
// Model discovery per provider.

func ListModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	var (
		names []string
		err   error
	)
	switch name {
	case "anthropic":
		names, err = anthropicModels(ctx, client, cfg)
	case "openai":
		names, err = openAIModels(ctx, client, cfg)
	case "google":
		names, err = googleModels(ctx, client, cfg)
	case "ollama":
		names, err = ollamaModels(ctx, client, cfg)
	case "mock":
		names = []string{"mock"}
	case "claudedesk", "codexdesk":
		names, err = bridgeModels(ctx, name, cfg)
	default:
		return nil, fmt.Errorf("provedor desconhecido: %s", name)
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// bridgeModels proves the CLI is installed (one --version run, no tokens) and lists what it offers.
func bridgeModels(ctx context.Context, name string, cfg ProviderConfig) ([]string, error) {
	kind := "claude"
	if name == "codexdesk" {
		kind = "codex"
	}
	cli := cfg.BaseURL
	if cli == "" {
		if kind == "codex" {
			cli = FindCodexCLI()
		} else {
			cli = FindClaudeCLI()
		}
	}
	if cli == "" {
		label := "o Claude Code"
		if kind == "codex" {
			label = "o Codex"
		}
		return nil, fmt.Errorf("%s não foi encontrado nesta máquina; instale o aplicativo ou o CLI e tente de novo", label)
	}
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(c, cli, "--version")
	process.NoConsoleWindow(cmd)
	if _, err := cmd.Output(); err != nil && c.Err() != nil {
		return nil, fmt.Errorf("não foi possível executar %s: %v", cli, err)
	}
	return BridgeModels(kind), nil
}

func fetchJSON(ctx context.Context, client *http.Client, provider, url string, headers map[string]string) ([]byte, error) {
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
		return nil, httpError(provider, resp)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

func anthropicModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]string, error) {
	b, err := fetchJSON(ctx, client, "anthropic", orDefault(cfg.BaseURL, "https://api.anthropic.com")+"/v1/models?limit=100", map[string]string{"x-api-key": cfg.APIKey, "anthropic-version": "2023-06-01"})
	if err != nil {
		return nil, err
	}
	var r struct {
		Data []struct{ ID string } `json:"data"`
	}
	_ = json.Unmarshal(b, &r)
	var names []string
	for _, d := range r.Data {
		names = append(names, d.ID)
	}
	return names, nil
}

func openAIModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]string, error) {
	h := map[string]string{}
	if cfg.APIKey != "" {
		h["authorization"] = "Bearer " + cfg.APIKey
	}
	b, err := fetchJSON(ctx, client, "openai", orDefault(cfg.BaseURL, openAIDefaultBase)+"/v1/models", h)
	if err != nil {
		return nil, err
	}
	var r struct {
		Data []struct{ ID string } `json:"data"`
	}
	_ = json.Unmarshal(b, &r)
	var names []string
	for _, d := range r.Data {
		names = append(names, d.ID)
	}
	return names, nil
}

func googleModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]string, error) {
	b, err := fetchJSON(ctx, client, "google", orDefault(cfg.BaseURL, googleDefaultBase)+"/v1beta/models?pageSize=200", map[string]string{"x-goog-api-key": cfg.APIKey})
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
	var names []string
	for _, m := range r.Models {
		for _, me := range m.Methods {
			if me == "generateContent" {
				names = append(names, strings.TrimPrefix(m.Name, "models/"))
				break
			}
		}
	}
	return names, nil
}

func ollamaModels(ctx context.Context, client *http.Client, cfg ProviderConfig) ([]string, error) {
	b, err := fetchJSON(ctx, client, "ollama", orDefault(cfg.BaseURL, ollamaDefaultBase)+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("Ollama não respondeu (está rodando?): %w", err)
	}
	var r struct {
		Models []struct{ Name string } `json:"models"`
	}
	_ = json.Unmarshal(b, &r)
	var names []string
	for _, m := range r.Models {
		names = append(names, m.Name)
	}
	return names, nil
}
