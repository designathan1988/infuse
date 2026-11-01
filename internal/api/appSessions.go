package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
	"infuse/internal/config"
	"infuse/internal/workspace"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// appSessions handles one group of application routes.

func appSessions(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any, root, dir string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	path := r.URL.Path
	_ = q
	switch {
	case path == "/api/sessions/new":
		newSessionRoute(w, root, str, boolean)
		return true
	case path == "/api/sessions/search":
		sendJSON(w, 200, agent.SearchSessions(q["q"], 40))
		return true
	case path == "/api/sessions/all":
		sendJSON(w, 200, sessionsOfEveryProject(q["archived"] == "1"))
		return true
	case strings.HasPrefix(path, "/api/session/"):
		return sessionRoute(w, r, body, root, str, num, boolean)
	}
	return false
}

func newSessionRoute(w http.ResponseWriter, root string, str func(string) string, boolean func(string) bool) {
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		sendJSON(w, 400, map[string]any{"error": "escolha a pasta do projeto"})
		return
	}
	s := agent.NewSession(root)
	s.Mu.Lock()
	if v := str("mode"); v != "" {
		s.Mode = v
	}
	if v := str("provider"); v != "" {
		s.Provider = v
	}
	if v := str("model"); v != "" {
		s.Model = v
	}
	if v := str("effort"); v != "" {
		s.Effort = v
	}
	if v := str("title"); v != "" {
		s.Title = v
	}
	s.Mu.Unlock()
	if boolean("worktree") {
		wt, branch, err := workspace.CreateWorktree(root, str("title"))
		if err != nil {
			_ = os.Remove(filepath.Join(agent.SessionsDir(root), s.ID+".json"))
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return
		}
		s.Mu.Lock()
		s.Worktree, s.Branch, s.Cwd = wt, branch, wt
		s.Mu.Unlock()
	} else {
		s.Branch = strings.TrimSpace(workspace.GitOutput(root, "rev-parse", "--abbrev-ref", "HEAD"))
	}
	s.Save()
	rememberProjectMode(root, s.Mode)
	sendJSON(w, 200, s.Snapshot())
}

func sessionsOfEveryProject(archived bool) []map[string]any {
	out := []map[string]any{}
	for _, p := range config.LoadProjects() {
		for _, d := range agent.ListSessions(p.Root) {
			if d.Archived != archived {
				continue
			}
			out = append(out, map[string]any{"id": d.ID, "root": d.Root, "title": d.Title, "updated": d.Updated, "created": d.Created,
				"cost": d.Cost, "branch": d.Branch, "worktree": d.Worktree, "archived": d.Archived, "state": agent.LiveState(d.ID), "model": d.Model, "pinned": d.Pinned})
		}
	}
	return out
}

type sessionAPI struct {
	w       http.ResponseWriter
	r       *http.Request
	s       *agent.Session
	body    map[string]any
	root    string
	str     func(string) string
	num     func(string) int
	boolean func(string) bool
}

func sessionRoute(w http.ResponseWriter, r *http.Request, body map[string]any, root string, str func(string) string, num func(string) int, boolean func(string) bool) bool {
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/session/"), "/", 2)
	if len(parts) < 2 {
		return false
	}
	s, err := agent.LoadSession(root, parts[0])
	if err != nil {
		sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
		return true
	}
	c := &sessionAPI{w: w, r: r, s: s, body: body, root: root, str: str, num: num, boolean: boolean}
	switch action := parts[1]; action {
	case "settings":
		c.settings()
	case "pin", "archive", "unarchive":
		c.flags(action)
	case "rewind", "summarize", "fork", "pinmsg":
		c.history(action)
	case "btw", "review", "suggest", "context", "comments":
		c.assist(action)
	case "bans", "procs", "mission":
		c.runtime(action)
	default:
		return false
	}
	return true
}

func (c *sessionAPI) settings() {
	s := c.s
	s.Mu.Lock()
	if v := c.str("mode"); v != "" {
		s.Mode = v
	}
	if v := c.str("provider"); v != "" {
		s.Provider = v
	}
	if v := c.str("model"); v != "" {
		s.Model = v
	}
	if v := c.str("effort"); v != "" {
		s.Effort = v
	}
	if v := c.str("title"); v != "" {
		s.Title = v
	}
	s.Mu.Unlock()
	s.Save()
	if v := c.str("mode"); v != "" {
		rememberProjectMode(c.root, v)
	}
	snap := s.Snapshot()
	s.Emit("session", snap)
	sendJSON(c.w, 200, snap)
}

func (c *sessionAPI) flags(action string) {
	s := c.s
	switch action {
	case "pin":
		s.Mu.Lock()
		s.Pinned = c.boolean("pinned")
		s.Mu.Unlock()
		s.Save()
		sendJSON(c.w, 200, map[string]any{"ok": true})
	case "archive":
		if s.Worktree != "" && !c.boolean("force") {
			if st := strings.TrimSpace(workspace.GitOutput(s.Worktree, "status", "--porcelain")); st != "" {
				sendJSON(c.w, 200, map[string]any{"ok": false, "dirty": true, "error": "o worktree tem alterações não commitadas"})
				return
			}
		}
		s.Stop()
		s.Mu.Lock()
		s.Archived = true
		s.Mu.Unlock()
		s.Save()
		sendJSON(c.w, 200, map[string]any{"ok": true})
	case "unarchive":
		s.Mu.Lock()
		s.Archived = false
		s.Mu.Unlock()
		s.Save()
		sendJSON(c.w, 200, map[string]any{"ok": true})
	}
}

func (c *sessionAPI) history(action string) {
	s := c.s
	switch action {
	case "rewind":
		prompt, err := s.RewindTo(c.num("turn"), c.boolean("code"), c.boolean("conversation"))
		if err != nil {
			sendJSON(c.w, 409, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(c.w, 200, map[string]any{"session": s.Snapshot(), "prompt": prompt})
	case "summarize":
		if err := s.SummarizeUpTo(c.r.Context(), c.num("turn"), c.str("hint")); err != nil {
			sendJSON(c.w, 409, map[string]any{"error": err.Error()})
			return
		}
		sendJSON(c.w, 200, map[string]any{"session": s.Snapshot()})
	case "fork":
		n, err := s.Fork(c.num("upTo"))
		if err != nil {
			sendJSON(c.w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		sendJSON(c.w, 200, map[string]any{"ok": true, "id": n.ID, "root": n.Root})
	case "pinmsg":
		sendJSON(c.w, 200, map[string]any{"ok": true, "pinned": s.TogglePin(c.num("index"))})
	}
}

func (c *sessionAPI) assist(action string) {
	s := c.s
	switch action {
	case "btw":
		ans, err := s.SideQuestion(c.r.Context(), c.str("question"))
		if err != nil {
			sendJSON(c.w, 200, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		sendJSON(c.w, 200, map[string]any{"ok": true, "answer": ans})
	case "review":
		out, err := s.IndependentReview(c.r.Context(), c.str("focus"))
		sendJSON(c.w, 200, map[string]any{"ok": err == nil, "review": out, "error": errString(err)})
	case "suggest":
		text, err := s.SuggestNext(c.r.Context())
		sendJSON(c.w, 200, map[string]any{"text": text, "error": errString(err)})
	case "context":
		sendJSON(c.w, 200, s.ContextBreakdown())
	case "comments":
		c.reviewComments()
	}
}

func (c *sessionAPI) reviewComments() {
	var items []struct {
		Path string `json:"path"`
		Line int    `json:"line"`
		Code string `json:"code"`
		Text string `json:"text"`
	}
	b, _ := json.Marshal(c.body["comments"])
	_ = json.Unmarshal(b, &items)
	if len(items) == 0 {
		sendJSON(c.w, 400, map[string]any{"error": "nenhum comentário"})
		return
	}
	var sb strings.Builder
	sb.WriteString("Comentários de revisão no diff. Aplique cada um e mostre o resultado:\n")
	for _, it := range items {
		fmt.Fprintf(&sb, "\n- %s:%d  `%s`\n  %s\n", it.Path, it.Line, strings.TrimSpace(it.Code), it.Text)
	}
	if err := c.s.Send(sb.String(), nil, nil); err != nil {
		sendJSON(c.w, 409, map[string]any{"error": err.Error()})
		return
	}
	sendJSON(c.w, 200, map[string]any{"ok": true})
}

func (c *sessionAPI) runtime(action string) {
	s := c.s
	switch action {
	case "bans":
		if c.str("action") == "set" {
			off, _ := c.body["off"].(bool)
			sendJSON(c.w, 200, map[string]any{"bans": s.LiftBan(c.num("index"), off)})
			return
		}
		sendJSON(c.w, 200, map[string]any{"bans": s.Bans()})
	case "procs":
		c.procs()
	case "mission":
		var err error
		if c.str("action") == "stop" {
			s.StopMission()
		} else {
			budget, _ := strconv.ParseFloat(fmt.Sprint(c.body["budget"]), 64)
			turns, _ := strconv.Atoi(fmt.Sprint(c.body["maxTurns"]))
			err = s.StartMission(c.str("goal"), budget, turns)
		}
		sendJSON(c.w, 200, map[string]any{"ok": err == nil, "error": errString(err), "mission": s.MissionSnapshot()})
	}
}

func (c *sessionAPI) procs() {
	s := c.s
	switch c.str("action") {
	case "stop":
		err := s.StopProc(c.str("id"))
		sendJSON(c.w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
	case "output":
		text, ok := s.ProcOutput(c.str("id"))
		if !ok {
			sendJSON(c.w, 404, map[string]any{"error": "processo não encontrado"})
			return
		}
		sendJSON(c.w, 200, map[string]any{"output": text})
	default:
		sendJSON(c.w, 200, map[string]any{"procs": s.ProcList()})
	}
}
