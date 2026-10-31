package agent

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const ddgPage = `<div class="result"><h2 class="result__title"><a rel="nofollow" class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fgo.dev%2Fdoc%2F&amp;rut=x">Documentation - <b>The Go</b> Programming Language</a></h2>
<a class="result__snippet" href="x">The Go programming language &amp; docs.</a><div class="clear"></div></div>
<div class="result"><a rel="nofollow" class="result__a" href="https://duckduckgo.com/y.js?ad=1">Ad</a><div class="clear"></div></div>`

const bingPage = `<ol><li class="b_algo" data-x="1"><h2><a href="https://pkg.go.dev/net/http" h="ID">http package - net/http</a></h2><div class="b_caption"><p class="b_lineclamp2">Package http provides HTTP client &amp; server.</p></div></li></ol>`

func TestSearchParsers(t *testing.T) {
	r := parseDDG(ddgPage)
	if len(r) != 1 || r[0].URL != "https://go.dev/doc/" || r[0].Title != "Documentation - The Go Programming Language" || r[0].Snippet != "The Go programming language & docs." {
		t.Fatalf("%+v", r)
	}
	if got := bingTarget("https://www.bing.com/ck/a?!&&p=1&u=a1aHR0cHM6Ly9nby5kZXYv&ntb=1"); got != "https://go.dev/" {
		t.Fatal(got)
	}
	b := parseBing(bingPage)
	if len(b) != 1 || b[0].URL != "https://pkg.go.dev/net/http" || !strings.Contains(b[0].Snippet, "client & server") {
		t.Fatalf("%+v", b)
	}
}

func TestWebSearchFallsBackToSecondEngine(t *testing.T) {
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>nada</html>") }))
	defer empty.Close()
	bing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "net http" {
			t.Errorf("query %q", r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, bingPage)
	}))
	defer bing.Close()
	saved := searchEngines
	defer func() { searchEngines = saved }()
	searchEngines = []searchEngine{
		{name: "a", url: func(q string) string { return empty.URL + "/?q=" + q }, parse: parseDDG},
		{name: "b", url: func(q string) string { return bing.URL + "/?q=" + strings.ReplaceAll(q, " ", "+") }, parse: parseBing},
	}
	res, engine, err := WebSearch(context.Background(), "net http", 5)
	if err != nil || engine != "b" || len(res) != 1 {
		t.Fatalf("%v %s %+v", err, engine, res)
	}
}
