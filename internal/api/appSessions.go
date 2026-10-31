package api

import (
	"encoding/json"
	"fmt"
	"infuse/internal/agent"
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
	// ---------------------------------------------------------------- sessions
	case path == "/api/sessions/new":
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "escolha a pasta do projeto"})
			return true
		}
		s := newSession(root)
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
		s.Mu.Unlock()
		if boolean("worktree") {
			wt, branch, err := CreateWorktree(root, str("title"))
			if err != nil {
				_ = os.Remove(filepath.Join(sessionsDir(root), s.ID+".json"))
				sendJSON(w, 400, map[string]any{"error": err.Error()})
				return true
			}
			s.Mu.Lock()
			s.Worktree, s.Branch, s.Cwd = wt, branch, wt
			s.Mu.Unlock()
		} else {
			s.Branch = strings.TrimSpace(gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD"))
		}
		s.Save()
		rememberProjectMode(root, s.Mode)
		sendJSON(w, 200, s.Snapshot())
		return true
	case path == "/api/sessions/search":
		sendJSON(w, 200, searchSessions(q["q"], 40))
		return true
	case path == "/api/sessions/all":
		// every session of every project, for the sidebar (grouping and sorting happen in the page)
		out := []map[string]any{}
		for _, p := range loadProjects() {
			for _, d := range listSessions(p.Root) {
				if d.Archived != (q["archived"] == "1") {
					continue
				}
				state := agent.LiveState(d.ID)
				out = append(out, map[string]any{"id": d.ID, "root": d.Root, "title": d.Title, "updated": d.Updated, "created": d.Created,
					"cost": d.Cost, "branch": d.Branch, "worktree": d.Worktree, "archived": d.Archived, "state": state, "model": d.Model, "pinned": d.Pinned})
			}
		}
		sendJSON(w, 200, out)
		return true
	case strings.HasPrefix(path, "/api/session/"):
		parts := strings.SplitN(strings.TrimPrefix(path, "/api/session/"), "/", 2)
		if len(parts) < 2 {
			return false
		}
		s, err := loadSession(root, parts[0])
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
			return true
		}
		switch parts[1] {
		case "settings": // mode, provider, model, effort, title
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
			s.Save()
			if v := str("mode"); v != "" {
				rememberProjectMode(root, v)
			}
			snap := s.Snapshot()
			s.Emit("session", snap)
			sendJSON(w, 200, snap)
		case "pin":
			s.Mu.Lock()
			s.Pinned = boolean("pinned")
			s.Mu.Unlock()
			s.Save()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "archive":
			force := boolean("force")
			if s.Worktree != "" && !force {
				if st := strings.TrimSpace(gitOutput(s.Worktree, "status", "--porcelain")); st != "" {
					sendJSON(w, 200, map[string]any{"ok": false, "dirty": true, "error": "o worktree tem alterações não commitadas"})
					return true
				}
			}
			s.Stop()
			s.Mu.Lock()
			s.Archived = true
			s.Mu.Unlock()
			s.Save()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "unarchive":
			s.Mu.Lock()
			s.Archived = false
			s.Mu.Unlock()
			s.Save()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "rewind":
			prompt, err := s.RewindTo(num("turn"), boolean("code"), boolean("conversation"))
			if err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"session": s.Snapshot(), "prompt": prompt})
		case "summarize":
			if err := s.SummarizeUpTo(r.Context(), num("turn"), str("hint")); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"session": s.Snapshot()})
		case "btw":
			ans, err := s.SideQuestion(r.Context(), str("question"))
			if err != nil {
				sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "answer": ans})
		case "fork": // a new session from this point of the conversation
			n, err := s.Fork(num("upTo"))
			if err != nil {
				sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true, "id": n.ID, "root": n.Root})
		case "pinmsg":
			sendJSON(w, 200, map[string]any{"ok": true, "pinned": s.TogglePin(num("index"))})
		case "bans": // the user's prohibitions: list, lift or restore one
			if str("action") == "set" {
				off, _ := body["off"].(bool)
				sendJSON(w, 200, map[string]any{"bans": s.LiftBan(num("index"), off)})
				return true
			}
			sendJSON(w, 200, map[string]any{"bans": s.Bans()})
		case "procs": // background processes: list, full output of one, stop one
			switch str("action") {
			case "stop":
				err := s.StopProc(str("id"))
				sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
			case "output":
				text, ok := s.ProcOutput(str("id"))
				if !ok {
					sendJSON(w, 404, map[string]any{"error": "processo não encontrado"})
					return true
				}
				sendJSON(w, 200, map[string]any{"output": text})
			default:
				sendJSON(w, 200, map[string]any{"procs": s.ProcList()})
			}
		case "mission":
			var err error
			if str("action") == "stop" {
				s.StopMission()
			} else {
				budget, _ := strconv.ParseFloat(fmt.Sprint(body["budget"]), 64)
				turns, _ := strconv.Atoi(fmt.Sprint(body["maxTurns"]))
				err = s.StartMission(str("goal"), budget, turns)
			}
			sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "mission": s.MissionSnapshot()})
		case "review": // independent reviewer on the current changes, asked by the user
			out, err := s.IndependentReview(r.Context(), str("focus"))
			sendJSON(w, 200, map[string]any{"ok": err == nil, "review": out, "error": errString(err)})
		case "suggest": // the gray suggestion in the prompt box
			text, err := s.SuggestNext(r.Context())
			sendJSON(w, 200, map[string]any{"text": text, "error": errString(err)})
		case "context":
			sendJSON(w, 200, s.ContextBreakdown())
		case "comments": // review comments from the diff pane become one message to the agent
			var items []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Code string `json:"code"`
				Text string `json:"text"`
			}
			b, _ := json.Marshal(body["comments"])
			_ = json.Unmarshal(b, &items)
			if len(items) == 0 {
				sendJSON(w, 400, map[string]any{"error": "nenhum comentário"})
				return true
			}
			var sb strings.Builder
			sb.WriteString("Comentários de revisão no diff. Aplique cada um e mostre o resultado:\n")
			for _, it := range items {
				fmt.Fprintf(&sb, "\n- %s:%d  `%s`\n  %s\n", it.Path, it.Line, strings.TrimSpace(it.Code), it.Text)
			}
			if err := s.Send(sb.String(), nil, nil); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"ok": true})
		default:
			return false
		}
		return true
	}
	return false
}
