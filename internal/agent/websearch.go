package agent

// Web search without a search API or key: the engines' plain HTML result pages are read and parsed
// (DuckDuckGo's HTML endpoint first, Bing when it returns nothing). Used by the agent's WebSearch tool
// and by the browser pane when it cannot show the engine's own page.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

type SearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type searchEngine struct {
	name  string
	url   func(q string) string
	parse func(page string) []SearchResult
}

const browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36"

var (
	reDDGResult  = regexp.MustCompile(`(?s)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>(.*?)(?:<a[^>]+class="result__snippet"[^>]*>(.*?)</a>|<div class="clear")`)
	reBingResult = regexp.MustCompile(`(?s)<li class="b_algo"[^>]*>.*?<h2[^>]*>\s*<a[^>]+href="([^"]+)"[^>]*>(.*?)</a>(.*?)</li>`)
	reBingSnip   = regexp.MustCompile(`(?s)<p[^>]*>(.*?)</p>`)
	reAnyTag     = regexp.MustCompile(`<[^>]+>`)

	searchEngines = []searchEngine{
		{name: "duckduckgo", url: func(q string) string { return "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q) }, parse: parseDDG},
		{name: "bing", url: func(q string) string { return "https://www.bing.com/search?q=" + url.QueryEscape(q) + "&setlang=pt-BR" }, parse: parseBing},
	}
)

func plainText(s string) string {
	return strings.Join(strings.Fields(html.UnescapeString(reAnyTag.ReplaceAllString(s, " "))), " ")
}

func parseDDG(page string) []SearchResult {
	var out []SearchResult
	for _, m := range reDDGResult.FindAllStringSubmatch(page, -1) {
		link := html.UnescapeString(m[1])
		if u, err := url.Parse(link); err == nil && strings.HasSuffix(u.Host, "duckduckgo.com") && u.Query().Get("uddg") != "" {
			link = u.Query().Get("uddg") // the redirect wrapper around the real address
		} else if strings.HasPrefix(link, "//") {
			link = "https:" + link
		}
		if strings.Contains(link, "duckduckgo.com/y.js") { // ads
			continue
		}
		out = append(out, SearchResult{Title: plainText(m[2]), URL: link, Snippet: plainText(m[4])})
	}
	return out
}

func parseBing(page string) []SearchResult {
	var out []SearchResult
	for _, m := range reBingResult.FindAllStringSubmatch(page, -1) {
		snip := ""
		if p := reBingSnip.FindStringSubmatch(m[3]); p != nil {
			snip = plainText(p[1])
		}
		out = append(out, SearchResult{Title: plainText(m[2]), URL: bingTarget(html.UnescapeString(m[1])), Snippet: snip})
	}
	return out
}

// bingTarget unwraps Bing's click-tracking link (/ck/a?...&u=a1<base64url of the address>).
func bingTarget(link string) string {
	u, err := url.Parse(link)
	if err != nil || !strings.HasSuffix(u.Host, "bing.com") || u.Path != "/ck/a" {
		return link
	}
	enc := strings.TrimPrefix(u.Query().Get("u"), "a1")
	if b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(enc, "=")); err == nil && strings.HasPrefix(string(b), "http") {
		return string(b)
	}
	return link
}

// WebSearch returns up to max results for q from the first engine that answers with any.
func WebSearch(ctx context.Context, q string, max int) ([]SearchResult, string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, "", errMsg(M("search.empty"))
	}
	if max <= 0 || max > 20 {
		max = 8
	}
	var lastErr error
	for _, e := range searchEngines {
		req, err := http.NewRequestWithContext(ctx, "GET", e.url(q), nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("User-Agent", browserUA)
		req.Header.Set("Accept-Language", "pt-BR,pt;q=0.9,en;q=0.8")
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			lastErr = errMsg(M("search.http", "engine", e.name, "status", resp.StatusCode))
			continue
		}
		res := e.parse(string(b))
		if len(res) == 0 {
			lastErr = errMsg(M("search.no_results", "engine", e.name))
			continue
		}
		if len(res) > max {
			res = res[:max]
		}
		return res, e.name, nil
	}
	return nil, "", lastErr
}

// ---------------------------------------------------------------- the agent's tool

type webSearchTool struct{}

func (webSearchTool) Category() string { return "net" }
func (webSearchTool) Spec() ToolSpec {
	return ToolSpec{Name: "WebSearch", Description: "Searches the web and returns the title, address and snippet of each result. Use it to find official documentation, error messages and versions; then read the page with WebFetch.",
		InputSchema: schema(`{"type":"object","properties":{"query":{"type":"string"},"max":{"type":"integer","description":"how many results (default 8)"}},"required":["query"]}`)}
}

func (webSearchTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Query string `json:"query"`
		Max   int    `json:"max"`
	}
	_ = json.Unmarshal(input, &in)
	Journal(tc.Root, JournalEntry{Kind: "fetch", Command: in.Query, Detail: "web search"})
	res, engine, err := WebSearch(ctx, in.Query, in.Max)
	if err != nil {
		return tc.Say(M("search.failed", "error", errArg(err))), true
	}
	var b strings.Builder
	for i, r := range res {
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return tc.clip(externalContent(engine+": "+in.Query, b.String())), false
}

// openDefaultBrowser hands an http(s) address to the system's default browser.
func openDefaultBrowser(u string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
	case "darwin":
		return exec.Command("open", u).Start()
	}
	return exec.Command("xdg-open", u).Start()
}

// frameable reports whether a site lets itself be shown inside another page (X-Frame-Options and
// CSP frame-ancestors). The browser pane uses it where it falls back to an iframe.
func frameable(ctx context.Context, u string) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return true
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return true // unreachable: let the frame show the browser's own error
	}
	resp.Body.Close()
	if xfo := strings.ToLower(resp.Header.Get("X-Frame-Options")); xfo == "deny" || xfo == "sameorigin" {
		return false
	}
	for _, csp := range resp.Header.Values("Content-Security-Policy") {
		for _, d := range strings.Split(strings.ToLower(csp), ";") {
			if f := strings.Fields(d); len(f) > 0 && f[0] == "frame-ancestors" && !strings.Contains(d, "*") {
				return false
			}
		}
	}
	return true
}
