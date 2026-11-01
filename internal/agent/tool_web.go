package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// WebFetch.

// ---------------------------------------------------------------- WebFetch

type webFetchTool struct{}

func (webFetchTool) Category() string { return "net" }
func (webFetchTool) Spec() ToolSpec {
	return ToolSpec{Name: "WebFetch", Description: "Downloads a web page or document and returns its text (HTML converted to text), in parts of 30000 characters. A page in parts must be read to the end (offset) before you run commands or change files.",
		InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string"},"offset":{"type":"integer","description":"where to continue reading (given at the end of each part)"}},"required":["url"]}`)}
}

var reTag = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
var reWs = regexp.MustCompile(`\n\s*\n+`)

func (webFetchTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		URL    string
		Offset int
	}
	_ = json.Unmarshal(input, &in)
	if bad, why := privateTargetMsg(in.URL); bad { // no requests into this machine or the private network
		Journal(tc.Root, JournalEntry{Kind: "blocked", Command: in.URL, Detail: why.Render("pt-BR")})
		return tc.Say(M("fetch.refused", "why", why)), true
	}
	Journal(tc.Root, JournalEntry{Kind: "fetch", Command: in.URL, Detail: "search"})
	req, err := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return err.Error(), true
	}
	req.Header.Set("User-Agent", "infuse/"+version)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err.Error(), true
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	text := string(b)
	if strings.Contains(resp.Header.Get("content-type"), "html") {
		text = reTag.ReplaceAllString(text, " ")
		text = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(text)
		text = reWs.ReplaceAllString(text, "\n\n")
	}
	text = strings.TrimSpace(text)
	total := max(1, (len(text)+docPageSize-1)/docPageSize)
	part := min(max(in.Offset, 0)/docPageSize, total-1)
	body := text[part*docPageSize : min(len(text), (part+1)*docPageSize)]
	head := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if total > 1 {
		head += " · " + tc.Say(M("fetch.part", "part", part+1, "total", total))
		if part+1 < total {
			body += "\n\n" + tc.Say(M("fetch.continues", "offset", (part+1)*docPageSize))
		}
	}
	if tc.S != nil && resp.StatusCode < 400 {
		tc.S.docs.markPart(in.URL, part, total)
	}
	return externalContent(in.URL, head+"\n\n"+body), resp.StatusCode >= 400
}
