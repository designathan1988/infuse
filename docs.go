package main

// Pinned documentation: pages the user pins (or picks from suggestions based on the project's
// dependencies) are fetched as text into .infuse/docs, and the agent is told to read them before using
// those libraries — the official docs of the version in use instead of what the model remembers.
// Pages are rendered by the headless browser when one is installed, so pages built with JavaScript work.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type PinnedDoc struct {
	URL     string `json:"url"`
	Title   string `json:"title"`
	File    string `json:"file"` // relative to the project
	Fetched int64  `json:"fetched"`
	Bytes   int    `json:"bytes"`
}

func docsDir(root string) string { return filepath.Join(root, infuseDirName, "docs") }

func loadDocs(root string) []PinnedDoc {
	var d []PinnedDoc
	_ = readJSON(filepath.Join(docsDir(root), "index.json"), &d)
	if d == nil {
		d = []PinnedDoc{}
	}
	return d
}

func saveDocs(root string, d []PinnedDoc) error {
	_ = os.MkdirAll(docsDir(root), 0o755)
	return writeJSON(filepath.Join(docsDir(root), "index.json"), d)
}

var reSlug = regexp.MustCompile(`[^a-z0-9]+`)

// fetchPageText renders url in the headless browser (or, without one, downloads it) and returns title and text.
func fetchPageText(ctx context.Context, root, url string) (string, string, error) {
	if b, err := agentBrowser(root); err == nil {
		if _, err := b.call(ctx, "Page.navigate", map[string]any{"url": url}); err == nil {
			b.waitLoad(ctx)
			time.Sleep(500 * time.Millisecond)
			v, err := b.eval(ctx, `JSON.stringify({ t: document.title, x: (document.querySelector('main, article, [role=main]') || document.body).innerText })`)
			if err == nil {
				var s string
				_ = json.Unmarshal(v, &s)
				var r struct{ T, X string }
				if json.Unmarshal([]byte(s), &r) == nil && strings.TrimSpace(r.X) != "" {
					return r.T, r.X, nil
				}
			}
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", browserUA)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	page := string(b)
	title := ""
	if m := regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`).FindStringSubmatch(page); m != nil {
		title = plainText(m[1])
	}
	page = regexp.MustCompile(`(?is)<(script|style|nav|footer)[^>]*>.*?</(script|style|nav|footer)>`).ReplaceAllString(page, " ")
	return title, plainText(page), nil
}

// PinDoc fetches url and keeps it in .infuse/docs (an existing pin of the same URL is refreshed).
func PinDoc(ctx context.Context, root, url string) (PinnedDoc, error) {
	url = strings.TrimSpace(url)
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return PinnedDoc{}, errors.New("informe um endereço http(s)")
	}
	title, text, err := fetchPageText(ctx, root, url)
	if err != nil {
		return PinnedDoc{}, err
	}
	if len(text) > 300<<10 {
		text = text[:300<<10] + "\n[… cortado]"
	}
	slug := strings.Trim(reSlug.ReplaceAllString(strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")), "-"), "-")
	if len(slug) > 80 {
		slug = slug[:80]
	}
	rel := filepath.ToSlash(filepath.Join(infuseDirName, "docs", slug+".md"))
	content := fmt.Sprintf("# %s\n\nFonte: %s · obtido em %s\n\n%s\n", firstNonEmpty(title, url), url, time.Now().Format("2006-01-02"), text)
	_ = os.MkdirAll(docsDir(root), 0o755)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
		return PinnedDoc{}, err
	}
	doc := PinnedDoc{URL: url, Title: firstNonEmpty(title, url), File: rel, Fetched: time.Now().UnixMilli(), Bytes: len(content)}
	docs := loadDocs(root)
	replaced := false
	for i := range docs {
		if docs[i].URL == url {
			docs[i], replaced = doc, true
		}
	}
	if !replaced {
		docs = append(docs, doc)
	}
	return doc, saveDocs(root, docs)
}

func unpinDoc(root, url string) error {
	docs := loadDocs(root)
	out := docs[:0]
	for _, d := range docs {
		if d.URL == url {
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(d.File)))
			continue
		}
		out = append(out, d)
	}
	return saveDocs(root, out)
}

func docsPrompt(root string) string {
	var b strings.Builder
	for _, d := range loadDocs(root) {
		fmt.Fprintf(&b, "- %s — %s → %s\n", d.Title, d.URL, d.File)
	}
	return b.String()
}

// official documentation of common dependencies, offered as one-click pins
var docsForDependency = map[string][2]string{
	"react": {"React", "https://react.dev/reference/react"}, "next": {"Next.js", "https://nextjs.org/docs"}, "vue": {"Vue", "https://vuejs.org/guide/introduction.html"},
	"svelte": {"Svelte", "https://svelte.dev/docs"}, "vite": {"Vite", "https://vite.dev/guide/"}, "express": {"Express", "https://expressjs.com/en/5x/api.html"},
	"tailwindcss": {"Tailwind CSS", "https://tailwindcss.com/docs/installation"}, "typescript": {"TypeScript", "https://www.typescriptlang.org/docs/handbook/intro.html"},
	"vitest": {"Vitest", "https://vitest.dev/api/"}, "jest": {"Jest", "https://jestjs.io/docs/api"}, "@playwright/test": {"Playwright", "https://playwright.dev/docs/api/class-test"},
	"prisma": {"Prisma", "https://www.prisma.io/docs/orm/reference/prisma-client-reference"}, "zod": {"Zod", "https://zod.dev/api"}, "@angular/core": {"Angular", "https://angular.dev/overview"},
	"fastify": {"Fastify", "https://fastify.dev/docs/latest/Reference/"}, "electron": {"Electron", "https://www.electronjs.org/docs/latest/api/app"},
	"django": {"Django", "https://docs.djangoproject.com/en/stable/"}, "flask": {"Flask", "https://flask.palletsprojects.com/en/stable/api/"}, "fastapi": {"FastAPI", "https://fastapi.tiangolo.com/reference/"},
	"pytest": {"pytest", "https://docs.pytest.org/en/stable/reference/reference.html"},
}

type docSuggestion struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Why  string `json:"why"`
}

// SuggestDocs lists official docs for the project's dependencies that are not pinned yet.
func SuggestDocs(root string) []docSuggestion {
	pinned := map[string]bool{}
	for _, d := range loadDocs(root) {
		pinned[d.URL] = true
	}
	out := []docSuggestion{}
	seen := map[string]bool{}
	add := func(dep, version string) {
		if d, ok := docsForDependency[dep]; ok && !pinned[d[1]] && !seen[d[1]] {
			seen[d[1]] = true
			out = append(out, docSuggestion{Name: d[0], URL: d[1], Why: strings.TrimSpace("dependência " + dep + " " + version)})
		}
	}
	var pkg struct{ Dependencies, DevDependencies map[string]string }
	if readJSON(filepath.Join(root, "package.json"), &pkg) == nil {
		for k, v := range pkg.Dependencies {
			add(k, v)
		}
		for k, v := range pkg.DevDependencies {
			add(k, v)
		}
	}
	for _, f := range []string{"requirements.txt", "pyproject.toml"} {
		if b, err := os.ReadFile(filepath.Join(root, f)); err == nil {
			low := strings.ToLower(string(b))
			for _, dep := range []string{"django", "flask", "fastapi", "pytest"} {
				if strings.Contains(low, dep) {
					add(dep, "")
				}
			}
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
		for _, m := range regexp.MustCompile(`(?m)^\s*(?:require\s+)?([a-z0-9.\-]+\.[a-z]+/[^\s]+)\s+(v[^\s]+)`).FindAllStringSubmatch(string(b), 8) {
			u := "https://pkg.go.dev/" + m[1] + "@" + m[2]
			if !pinned[u] {
				out = append(out, docSuggestion{Name: m[1], URL: u, Why: "módulo Go " + m[2]})
			}
		}
	}
	return out
}
