package agent

// WebSearch's provider table: the built-in engines (the default chain) plus the JSON APIs of the user's
// settings. A provider comes from the user's layer alone: a project file cannot send the agent's
// searches to an address of its choosing.

import (
	"context"
	"encoding/json"
	"infuse/internal/config"
	"net/url"
	"strings"
)

// SearchProviderNames lists what the searchProvider setting can name.
func SearchProviderNames(s Settings) []string {
	names := make([]string, 0, len(searchEngines)+len(s.SearchProviders))
	for _, e := range searchEngines {
		names = append(names, e.name)
	}
	for _, p := range s.SearchProviders {
		names = append(names, p.Name)
	}
	return names
}

// webSearch runs a query through the session's provider; "" is the default chain.
func (s *Session) webSearch(ctx context.Context, q string, max int) ([]SearchResult, string, error) {
	name := s.settings.SearchProvider
	if name == "" {
		return WebSearch(ctx, q, max)
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, "", errMsg(M("search.empty"))
	}
	if max <= 0 || max > 20 {
		max = 8
	}
	engine, err := providerEngine(s.settings, name)
	if err != nil {
		return nil, "", err
	}
	return searchWith(ctx, []searchEngine{engine}, q, max)
}

func providerEngine(st Settings, name string) (searchEngine, error) {
	for _, e := range searchEngines {
		if e.name == name {
			return e, nil
		}
	}
	for _, p := range st.SearchProviders {
		if p.Name == name {
			return customEngine(p)
		}
	}
	return searchEngine{}, errMsg(M("search.unknown_provider", "name", name))
}

// customEngine turns a settings row into an engine. Its key stays in the vault until the request.
func customEngine(p config.SearchProvider) (searchEngine, error) {
	e := searchEngine{name: p.Name, parse: func(page string) []SearchResult { return parseJSONResults(p, page) }}
	e.url = func(q string) string { return strings.ReplaceAll(p.URL, "{query}", url.QueryEscape(q)) }
	if p.Header != "" {
		key, err := config.SecretGet(p.KeyVault)
		if err != nil || key == "" {
			return e, errMsg(M("search.no_key", "name", p.Name))
		}
		e.headers = map[string]string{p.Header: key}
	}
	return e, nil
}

// parseJSONResults reads a provider's answer through the row's field names.
func parseJSONResults(p config.SearchProvider, page string) []SearchResult {
	var doc any
	if json.Unmarshal([]byte(page), &doc) != nil {
		return nil
	}
	for _, key := range strings.Split(p.Results, ".") {
		m, ok := doc.(map[string]any)
		if !ok {
			return nil
		}
		doc = m[key]
	}
	items, _ := doc.([]any)
	var out []SearchResult
	for _, it := range items {
		row, _ := it.(map[string]any)
		title, _ := row[p.Title].(string)
		link, _ := row[p.Link].(string)
		snippet, _ := row[p.Snippet].(string)
		if link != "" {
			out = append(out, SearchResult{Title: plainText(title), URL: link, Snippet: plainText(snippet)})
		}
	}
	return out
}
