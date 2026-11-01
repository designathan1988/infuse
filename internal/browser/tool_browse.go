package browser

import (
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/security"
	"strings"
	"time"
)

// Navigation and history.

func (a *action) browse() (string, bool) {
	switch a.in.Action {
	case "navigate":
		u := strings.TrimSpace(a.in.URL)
		if u == "" {
			return a.h.Say(core.M("browser.give_url")), true
		}
		if !strings.Contains(u, "://") && !strings.HasPrefix(u, "about:") {
			if strings.HasPrefix(u, "localhost") || strings.HasPrefix(u, "127.") {
				u = "http://" + u
			} else {
				u = "https://" + u
			}
		}
		if bad, why := security.BrowserTargetMsg(u); bad {
			journal.Journal(a.h.RootDir(), journal.JournalEntry{Kind: "blocked", Command: u, Detail: why.String()})
			return a.h.Say(core.M("fetch.refused", "why", why)), true
		}
		journal.Journal(a.h.RootDir(), journal.JournalEntry{Kind: "fetch", Command: u, Detail: "agent browser"})
		raw, err := a.p.b.call(a.ctx, "Page.navigate", map[string]any{"url": u})
		if err != nil {
			return a.h.Say(core.M("browser.open_failed", "error", core.ErrArg(err))), true
		}
		var nav struct {
			ErrorText string `json:"errorText"`
		}
		_ = json.Unmarshal(raw, &nav)
		if nav.ErrorText != "" {
			return a.h.Say(core.M("browser.open_failed_url", "url", u, "error", nav.ErrorText)), true
		}
		a.p.b.waitLoad(a.ctx)
		a.showURL()
		return a.h.Say(core.M("browser.opened", "where", a.where())), false
	case "back", "reload":
		if a.in.Action == "back" {
			_, _ = a.p.b.eval(a.ctx, "history.back()")
		} else {
			_, _ = a.p.b.call(a.ctx, "Page.reload", nil)
		}
		sleepCtx(a.ctx, 400*time.Millisecond)
		a.p.b.waitLoad(a.ctx)
		a.showURL()
		if a.in.Action == "back" {
			return a.h.Say(core.M("browser.went_back", "where", a.where())), false
		}
		return a.h.Say(core.M("browser.reloaded", "where", a.where())), false
	}
	return "", true
}
