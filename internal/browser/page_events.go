package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"strings"
	"time"
)

// CDP event handling: network, console, frames, navigation.

func (p *Page) event(method string, params json.RawMessage) {
	switch method {
	case "Network.requestWillBeSent":
		var e struct {
			RequestID string                       `json:"requestId"`
			Type      string                       `json:"type"`
			Request   struct{ Method, URL string } `json:"request"`
		}
		_ = json.Unmarshal(params, &e)
		n := &netEntry{ID: e.RequestID, Method: e.Request.Method, URL: e.Request.URL, Type: e.Type, start: time.Now()}
		p.mu.Lock()
		p.net = append(p.net, n)
		if len(p.net) > 400 {
			delete(p.netByID, p.net[0].ID)
			p.net = p.net[1:]
		}
		p.netByID[e.RequestID] = n
		p.mu.Unlock()
	case "Network.responseReceived":
		var e struct {
			RequestID string `json:"requestId"`
			Response  struct {
				Status   int    `json:"status"`
				MimeType string `json:"mimeType"`
			} `json:"response"`
		}
		_ = json.Unmarshal(params, &e)
		p.mu.Lock()
		if n := p.netByID[e.RequestID]; n != nil {
			n.Status, n.Mime = e.Response.Status, e.Response.MimeType
		}
		p.mu.Unlock()
	case "Network.loadingFinished", "Network.loadingFailed":
		var e struct {
			RequestID         string  `json:"requestId"`
			EncodedDataLength float64 `json:"encodedDataLength"`
			ErrorText         string  `json:"errorText"`
		}
		_ = json.Unmarshal(params, &e)
		p.mu.Lock()
		n := p.netByID[e.RequestID]
		if n != nil {
			n.Bytes, n.Failed, n.finished = e.EncodedDataLength, e.ErrorText, true
			n.Ms = time.Since(n.start).Milliseconds()
		}
		p.mu.Unlock()
		if n != nil && (n.Failed != "" || n.Status >= 400) {
			p.note(core.M("browser.ev_network", "method", n.Method, "url", core.Truncate(n.URL, 120), "result", core.FirstNonEmpty(n.Failed, fmt.Sprint(n.Status))))
		}
	case "Page.screencastFrame":
		var f struct {
			Data      string `json:"data"`
			SessionID int    `json:"sessionId"`
		}
		_ = json.Unmarshal(params, &f)
		go func() {
			_, _ = p.b.call(context.Background(), "Page.screencastFrameAck", map[string]any{"sessionId": f.SessionID})
		}()
		if j, err := base64.StdEncoding.DecodeString(f.Data); err == nil {
			p.mu.Lock()
			if p.rec && len(p.frames) < 3000 {
				p.frames = append(p.frames, recFrame{time.Since(p.recStart), j})
			}
			p.mu.Unlock()
		}
	case "Runtime.exceptionThrown":
		var e struct {
			Details struct {
				Text      string `json:"text"`
				Exception struct {
					Description string `json:"description"`
				} `json:"exception"`
			} `json:"exceptionDetails"`
		}
		_ = json.Unmarshal(params, &e)
		p.note(core.M("browser.ev_exception", "text", core.Truncate(core.FirstLine(core.FirstNonEmpty(e.Details.Exception.Description, e.Details.Text)), 200)))
	case "Runtime.consoleAPICalled":
		var e struct {
			Type string `json:"type"`
			Args []struct {
				Value       any    `json:"value"`
				Description string `json:"description"`
			} `json:"args"`
		}
		_ = json.Unmarshal(params, &e)
		if e.Type == "error" || e.Type == "warning" {
			var parts []string
			for _, a := range e.Args {
				parts = append(parts, core.FirstNonEmpty(fmt.Sprint(a.Value), a.Description))
			}
			p.note(core.M("browser.ev_console", "type", e.Type, "text", core.Truncate(strings.Join(parts, " "), 200)))
		}
	case "Page.frameNavigated":
		var e struct {
			Frame struct {
				ParentID string `json:"parentId"`
				URL      string `json:"url"`
			} `json:"frame"`
		}
		_ = json.Unmarshal(params, &e)
		if e.Frame.ParentID == "" {
			p.note(core.M("browser.ev_navigated", "url", core.Truncate(e.Frame.URL, 160)))
		}
	case "Page.loadEventFired":
		p.mu.Lock()
		x, y := p.x, p.y
		p.mu.Unlock()
		if x >= 0 {
			go func() {
				_, _ = p.b.eval(context.Background(), fmt.Sprintf("window.__infuseCursor && __infuseCursor.move(%f,%f)", x, y))
			}()
		}
	}
}

// ---------------------------------------------------------------- human gestures
