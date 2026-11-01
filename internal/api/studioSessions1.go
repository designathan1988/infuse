package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/core"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func legacySessions(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string) {
	if r.URL.Path == "/api/sessions" {
		legacySessionRoot(w, r, root)
		return
	}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/", 2)
	s, err := agent.LoadSession(root, parts[0])
	if err != nil {
		sendJSON(w, 404, map[string]any{"error": "session not found", "code": "session.notFound"})
		return
	}
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}
	c := &legacySessionAPI{w: w, r: r, s: s, body: body, root: root, str: str}
	switch action {
	case "":
		c.rootRoute()
	case "duplicate":
		c.duplicate()
	case "send":
		c.send()
	case "stop", "permission", "answer", "revert", "events":
		c.control(action)
	default:
		http.NotFound(w, r)
	}
}

type legacySessionAPI struct {
	w    http.ResponseWriter
	r    *http.Request
	s    *agent.Session
	body map[string]any
	root string
	str  func(string) string
}

func legacySessionRoot(w http.ResponseWriter, r *http.Request, root string) {
	if r.Method == http.MethodGet {
		sendJSON(w, 200, agent.ListSessions(root))
		return
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		sendJSON(w, 400, map[string]any{"error": "the project folder was not found", "code": "project.notFound"})
		return
	}
	s := agent.NewSession(root)
	sendJSON(w, 200, s.Snapshot())
}

func (c *legacySessionAPI) rootRoute() {
	switch c.r.Method {
	case http.MethodDelete:
		c.s.Stop()
		agent.RemoveSession(c.root, c.s.ID)
		sendJSON(c.w, 200, map[string]any{"ok": true})
	case http.MethodPut:
		c.s.Mu.Lock()
		if t := c.str("title"); t != "" {
			c.s.Title = t
		}
		if m := c.str("model"); m != "" {
			c.s.Model = m
		}
		c.s.Mu.Unlock()
		c.s.Save()
		sendJSON(c.w, 200, c.s.Snapshot())
	default:
		sendJSON(c.w, 200, c.s.Snapshot())
	}
}

func (c *legacySessionAPI) duplicate() {
	d := agent.NewSession(c.root)
	d.Mu.Lock()
	d.Title = c.s.Title + " (copy)"
	d.Model = c.s.Model
	d.Messages = c.s.CopyMessages()
	d.Turn = c.s.Turn
	d.TurnStarts = append([]int{}, c.s.TurnStarts...)
	d.Mu.Unlock()
	d.Save()
	sendJSON(c.w, 200, d.Snapshot())
}

func (c *legacySessionAPI) send() {
	var atts []core.Attachment
	ab, _ := json.Marshal(c.body["attachments"])
	_ = json.Unmarshal(ab, &atts)
	var imgs []core.InlineImage
	ib, _ := json.Marshal(c.body["images"])
	_ = json.Unmarshal(ib, &imgs)
	if err := c.s.Send(c.str("text"), atts, imgs); err != nil {
		sendJSON(c.w, 409, map[string]any{"error": err.Error()})
		return
	}
	sendJSON(c.w, 200, map[string]any{"ok": true, "turn": c.s.Turn})
}

func (c *legacySessionAPI) control(action string) {
	switch action {
	case "stop":
		c.s.Stop()
		sendJSON(c.w, 200, map[string]any{"ok": true})
	case "permission":
		sendJSON(c.w, 200, map[string]any{"ok": c.s.ResolvePermission(c.str("id"), c.str("decision"))})
	case "answer":
		sendJSON(c.w, 200, map[string]any{"ok": c.s.Answer(c.str("id"), c.str("answer"))})
	case "revert":
		t, _ := strconv.Atoi(fmt.Sprint(c.body["turn"]))
		if err := c.s.Revert(t); err != nil {
			sendJSON(c.w, 409, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(c.w, 200, c.s.Snapshot())
	case "events":
		streamEvents(c.w, c.r, c.s)
	}
}
