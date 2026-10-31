package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"strings"
)

// Network, accessibility and device diagnostics.

func (p *Page) networkReport(onlyIssues bool) (string, int) {
	p.mu.Lock()
	list := append([]*netEntry{}, p.net...)
	p.mu.Unlock()
	var b strings.Builder
	shown := 0
	for _, n := range list {
		issue := n.Failed != "" || n.Status >= 400
		if onlyIssues && !issue {
			continue
		}
		status := fmt.Sprint(n.Status)
		if n.Failed != "" {
			status = "FAILED " + n.Failed
		} else if !n.finished {
			status += " (pending)"
		}
		fmt.Fprintf(&b, "%s %s %s · %s · %d ms · %.1f KB\n", n.Method, status, core.Truncate(n.URL, 160), n.Type, n.Ms, n.Bytes/1024)
		shown++
	}
	return b.String(), shown
}

func (p *Page) axTree(ctx context.Context) string {
	raw, err := p.b.call(ctx, "Accessibility.getFullAXTree", nil)
	if err != nil {
		return err.Error()
	}
	var t struct {
		Nodes []struct {
			Ignored bool `json:"ignored"`
			Role    struct {
				Value string `json:"value"`
			} `json:"role"`
			Name struct {
				Value string `json:"value"`
			} `json:"name"`
		} `json:"nodes"`
	}
	_ = json.Unmarshal(raw, &t)
	var b strings.Builder
	count := 0
	named := map[string]bool{}
	for _, n := range t.Nodes {
		role, name := n.Role.Value, strings.TrimSpace(n.Name.Value)
		if n.Ignored || role == "generic" || role == "none" || role == "InlineTextBox" || role == "LabelText" || role == "MenuListPopup" {
			continue
		}
		if role == "StaticText" && (name == "" || named[name]) {
			continue
		}
		named[name] = true
		fmt.Fprintf(&b, "%s %q\n", role, core.Truncate(n.Name.Value, 100))
		if count++; count >= 400 {
			b.WriteString("…\n")
			break
		}
	}
	return b.String()
}

var devicePresets = map[string]struct {
	w, h   int
	scale  float64
	mobile bool
	ua     string
}{
	"iphone": {390, 844, 3, true, "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"},
	"pixel":  {412, 915, 2.625, true, "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36"},
	"ipad":   {820, 1180, 2, true, "Mozilla/5.0 (iPad; CPU OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"},
	"laptop": {1366, 768, 1, false, ""},
}
