package main

// Routes for the desktop-style window: providers, models, sessions (create with worktree, archive,
// rewind variants, side chat, context), review (changes, PR), files, editors, terminal, dev servers,
// scheduled tasks, usage totals.

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func appAPI(w http.ResponseWriter, r *http.Request, q map[string]string, body map[string]any) bool {
	root := q["root"]
	path := r.URL.Path
	str := func(k string) string { s, _ := body[k].(string); return s }
	num := func(k string) int { f, _ := body[k].(float64); return int(f) }
	boolean := func(k string) bool { b, _ := body[k].(bool); return b }
	if strings.HasPrefix(path, "/api/view/") && viewAPI(w, r, root, path, body) { // the streamed browser pane
		return true
	}
	// dir: where git and file operations run (a session's worktree), always inside the project.
	dir := root
	if d := q["dir"]; d != "" {
		if rel, err := filepath.Rel(root, d); err == nil && !strings.HasPrefix(rel, "..") {
			dir = d
		}
	}

	switch {
	// ---------------------------------------------------------------- providers and models
	case path == "/api/connections" && r.Method == http.MethodGet:
		s := LoadSettings()
		out := []map[string]any{}
		for _, p := range providerPresets {
			c := s.Connections[p.ID]
			base := c.BaseURL
			if base == "" {
				base = p.BaseURL
			}
			models := c.Models
			if models == nil {
				models = []string{}
			}
			out = append(out, map[string]any{"id": p.ID, "kind": p.Kind, "label": p.Label, "baseUrl": base, "defaultBaseUrl": p.BaseURL,
				"needKey": p.NeedKey, "keyUrl": p.KeyURL, "local": p.Local, "connected": c.Connected, "keyHint": c.KeyHint,
				"models": models, "default": s.Provider == p.ID})
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/connections/connect":
		c, err := ConnectProvider(r.Context(), str("id"), str("apiKey"), str("baseUrl"))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "connection": c})
		return true
	case path == "/api/connections/disconnect":
		_ = DisconnectProvider(str("id"))
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	case path == "/api/connections/default":
		s := LoadSettings()
		if id := str("id"); s.Connections[id].Connected {
			s.Provider = id
			if m := str("model"); m != "" {
				s.Model = m
			}
			_ = SaveSettings(s)
		}
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	// ---------------------------------------------------------------- the oracle's executor
	case path == "/api/oracle/spec" && r.Method == http.MethodGet:
		spec, err := LoadOracleSpec(root)
		if err != nil {
			sendJSON(w, 200, map[string]any{"error": err.Error(), "spec": OracleSpec{Checks: []OracleCheck{}}})
			return true
		}
		if spec.Checks == nil {
			spec.Checks = []OracleCheck{}
		}
		sendJSON(w, 200, map[string]any{"spec": spec})
		return true
	case path == "/api/oracle/spec" && r.Method == http.MethodPut:
		var spec OracleSpec
		b, _ := jsonMarshal(body["spec"])
		if err := jsonUnmarshal(b, &spec); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		_ = os.MkdirAll(filepath.Dir(oracleSpecFile(root)), 0o755)
		if err := writeJSON(oracleSpecFile(root), spec); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		Journal(root, JournalEntry{Kind: "oracle", Path: ".infuse/oracle/checks.json", Detail: "critérios editados pelo usuário"})
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	case path == "/api/oracle/run":
		// "Pronto?": only the user's window calls this (API token); no agent tool can.
		work := dir
		v, err := RunOracle(r.Context(), root, work, str("session"))
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error(), "verdict": v})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "verdict": v})
		return true
	case path == "/api/shot": // screenshots taken by the agent's browser
		name := filepath.Base(q["name"])
		if !strings.HasSuffix(name, ".png") {
			sendJSON(w, 400, map[string]any{"error": "nome inválido"})
			return true
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=86400")
		http.ServeFile(w, r, filepath.Join(root, infuseDirName, "shots", name))
		return true
	// ---------------------------------------------------------------- pinned docs and conventions
	case path == "/api/docs" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"docs": loadDocs(root), "suggestions": SuggestDocs(root)})
		return true
	case path == "/api/docs/pin":
		doc, err := PinDoc(r.Context(), root, str("url"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "doc": doc})
		return true
	case path == "/api/docs/unpin":
		err := unpinDoc(root, str("url"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/conventions" && r.Method == http.MethodGet:
		sendJSON(w, 200, map[string]any{"text": conventionsPrompt(root), "file": conventionsFile(root)})
		return true
	case path == "/api/conventions":
		var err error
		if boolean("relearn") {
			text := LearnConventions(root)
			_ = os.MkdirAll(filepath.Dir(conventionsFile(root)), 0o755)
			err = os.WriteFile(conventionsFile(root), []byte(text), 0o644)
		} else {
			err = os.WriteFile(conventionsFile(root), []byte(str("text")), 0o644)
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "text": conventionsPrompt(root)})
		return true
	// ---------------------------------------------------------------- hooks and skills
	case path == "/api/hooks" && r.Method == http.MethodGet:
		out := LoadHooks(root)
		if out == nil {
			out = []HookEntry{}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/hooks/add":
		ev, cmdline := str("event"), strings.TrimSpace(str("command"))
		valid := false
		for _, e := range hookEvents {
			valid = valid || e == ev
		}
		if !valid || cmdline == "" {
			sendJSON(w, 200, map[string]any{"ok": false, "error": "escolha o momento e escreva o comando"})
			return true
		}
		scope := str("scope")
		if scope != "global" {
			scope = "project"
		}
		err := addHook(root, scope, ev, str("matcher"), cmdline)
		if err == nil { // added by the user in the app: approved in the same click
			err = approveHook(hookKey(root, scope, ev, str("matcher"), cmdline))
		}
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/hooks/approve":
		sendJSON(w, 200, map[string]any{"ok": approveHook(str("key")) == nil})
		return true
	case path == "/api/hooks/remove":
		err := removeHook(root, str("key"))
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/skills" && r.Method == http.MethodGet:
		out := LoadSkills(root)
		for i := range out {
			if b, err := os.ReadFile(filepath.Join(out[i].Dir, "SKILL.md")); err == nil {
				_, _, out[i].Body = parseSkill(string(b))
			}
		}
		if out == nil {
			out = []Skill{}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/skills/save":
		name := strings.TrimSpace(str("name"))
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`).MatchString(name) {
			sendJSON(w, 200, map[string]any{"ok": false, "error": "nome: letras minúsculas, números e hífens (ex.: revisar-pr)"})
			return true
		}
		dir := filepath.Join(root, infuseDirName, "skills", name)
		if str("scope") == "global" {
			dir = filepath.Join(InfuseHome(), "skills", name)
		}
		desc := strings.ReplaceAll(strings.TrimSpace(str("description")), "\n", " ")
		content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + strings.TrimSpace(str("body")) + "\n"
		_ = os.MkdirAll(dir, 0o755)
		err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
		sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err)})
		return true
	case path == "/api/skills/remove":
		for _, s := range LoadSkills(root) {
			if s.Name == str("name") && (strings.HasPrefix(s.Dir, filepath.Join(root, infuseDirName)) || strings.HasPrefix(s.Dir, InfuseHome())) {
				_ = os.RemoveAll(s.Dir)
			}
		}
		sendJSON(w, 200, map[string]any{"ok": true})
		return true
	// ---------------------------------------------------------------- MCP connectors
	case path == "/api/mcp" && r.Method == http.MethodGet:
		sendJSON(w, 200, mcpFor(root).status())
		return true
	case strings.HasPrefix(path, "/api/mcp/"):
		scope, name := str("scope"), str("name")
		if scope != "global" {
			scope = "project"
		}
		m := mcpFor(root)
		var err error
		switch strings.TrimPrefix(path, "/api/mcp/") {
		case "add": // a server from a preset or written by the user
			var cfg mcpServerConfig
			b, _ := jsonMarshal(body["config"])
			if err = jsonUnmarshal(b, &cfg); err == nil {
				if strings.TrimSpace(name) == "" || (cfg.Command == "" && cfg.URL == "") {
					err = errors.New("informe o nome e o comando (ou o endereço) do servidor")
				} else {
					err = editMCP(root, scope, func(sv map[string]mcpServerConfig) { sv[name] = cfg })
				}
			}
			if err == nil && boolean("approve") { // added by the user from the app: approved in the same click
				err = approveMCP(root, scope, name, cfg)
			}
		case "approve":
			cfg, ok := loadMCPFile(mcpFilePath(root, scope)).Servers[name]
			if !ok {
				err = errors.New("servidor não encontrado")
			} else {
				err = approveMCP(root, scope, name, cfg)
			}
		case "toggle":
			err = editMCP(root, scope, func(sv map[string]mcpServerConfig) {
				if c, ok := sv[name]; ok {
					c.Disabled = boolean("disabled")
					sv[name] = c
				}
			})
		case "remove":
			err = editMCP(root, scope, func(sv map[string]mcpServerConfig) { delete(sv, name) })
		case "restart":
		default:
			sendJSON(w, 404, map[string]any{"error": "ação desconhecida"})
			return true
		}
		if err != nil {
			sendJSON(w, 200, map[string]any{"ok": false, "error": err.Error()})
			return true
		}
		m.restart(r.Context(), scope, name)
		sendJSON(w, 200, map[string]any{"ok": true, "servers": m.status()})
		return true
	// ---------------------------------------------------------------- structured memory (the user's curation)
	case path == "/api/memory/items" && r.Method == http.MethodGet:
		out := map[string]any{"project": loadMemory(projectMemoryFile(root)), "global": loadMemory(globalMemoryFile()), "session": []MemoryItem{}}
		if id := q["session"]; id != "" {
			if s, err := loadSession(root, id); err == nil {
				s.mu.Lock()
				out["session"] = append([]MemoryItem{}, s.Memory...)
				s.mu.Unlock()
			}
		}
		sendJSON(w, 200, out)
		return true
	case path == "/api/memory/items/save":
		var items []MemoryItem
		b, _ := jsonMarshal(body["items"])
		if err := jsonUnmarshal(b, &items); err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		clean := []MemoryItem{}
		for _, it := range items {
			if it.Origin == "" {
				it.Origin = "usuario"
			}
			if normalizeMemory(&it) == nil {
				clean = append(clean, it)
			}
		}
		var err error
		switch scope := str("scope"); scope {
		case "session":
			s, e := loadSession(root, str("session"))
			if e != nil {
				sendJSON(w, 404, map[string]any{"error": "sessão não encontrada"})
				return true
			}
			err = s.editMemory("session", func([]MemoryItem) []MemoryItem { return clean })
		case "global":
			err = saveMemory(globalMemoryFile(), clean)
		default:
			err = saveMemory(projectMemoryFile(root), clean)
		}
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "items": clean})
		return true
	case path == "/api/oracle/suggest":
		sendJSON(w, 200, SuggestChecks(root))
		return true
	case path == "/api/oracle/verdicts":
		sendJSON(w, 200, ListVerdicts(root, 20))
		return true
	case path == "/api/oracle/evidence":
		run, name := filepath.Base(q["run"]), filepath.Base(q["file"])
		b, err := os.ReadFile(filepath.Join(evidenceDir(root), run, name))
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": "evidência não encontrada"})
			return true
		}
		sendJSON(w, 200, map[string]any{"name": name, "content": string(b)})
		return true
	case path == "/api/apps/command":
		// the shell command that starts a CLI app, for the terminal pane
		for _, a := range DetectApps() {
			if a.ID != str("id") || a.Kind != "cli" {
				continue
			}
			if !a.Found { // not installed: the terminal runs the official installer, visibly
				if pkg := cliPackages[a.ID]; pkg != "" && str("install") != "" {
					sendJSON(w, 200, map[string]any{"ok": true, "command": "npm install -g " + pkg, "name": "instalar " + a.Name, "installing": true})
					return true
				}
				break
			}
			cmd := shellQuote(toBashPath(a.Path))
			if a.ID == "codex-cli" && strings.Contains(a.Path, "WindowsApps") {
				cmd += " --no-daemon" // the CLI bundled with the Codex app needs this outside the app's background server
			}
			sendJSON(w, 200, map[string]any{"ok": true, "command": cmd, "name": a.Name})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": false, "error": "aplicativo não encontrado"})
		return true
	case path == "/api/models":
		s := LoadSettings()
		changed := false
		for id, c := range s.Connections {
			if c.Connected && len(c.Models) == 0 && c.Kind != "mock" {
				kind, cfg := connectionConfig(s, id)
				if names, err := ListModels(r.Context(), kind, cfg); err == nil && len(names) > 0 {
					c.Models = names
					s.Connections[id] = c
					changed = true
				}
			}
		}
		if changed {
			_ = SaveSettings(s)
		}
		sendJSON(w, 200, AvailableModels(s))
		return true
	case path == "/api/usage":
		sendJSON(w, 200, UsageTotals())
		return true

	// ---------------------------------------------------------------- sessions
	case path == "/api/sessions/new":
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			sendJSON(w, 400, map[string]any{"error": "escolha a pasta do projeto"})
			return true
		}
		s := newSession(root)
		s.mu.Lock()
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
		s.mu.Unlock()
		if boolean("worktree") {
			wt, branch, err := CreateWorktree(root, str("title"))
			if err != nil {
				_ = os.Remove(filepath.Join(sessionsDir(root), s.ID+".json"))
				sendJSON(w, 400, map[string]any{"error": err.Error()})
				return true
			}
			s.mu.Lock()
			s.Worktree, s.Branch, s.Cwd = wt, branch, wt
			s.mu.Unlock()
		} else {
			s.Branch = strings.TrimSpace(gitOutput(root, "rev-parse", "--abbrev-ref", "HEAD"))
		}
		s.save()
		rememberProjectMode(root, s.Mode)
		sendJSON(w, 200, s.snapshot())
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
				state := "idle"
				sessionsMu.Lock()
				if live, ok := sessions[d.ID]; ok {
					state = live.State
				}
				sessionsMu.Unlock()
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
			s.mu.Lock()
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
			s.mu.Unlock()
			s.save()
			if v := str("mode"); v != "" {
				rememberProjectMode(root, v)
			}
			snap := s.snapshot()
			s.emit("session", snap)
			sendJSON(w, 200, snap)
		case "pin":
			s.mu.Lock()
			s.Pinned = boolean("pinned")
			s.mu.Unlock()
			s.save()
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
			s.mu.Lock()
			s.Archived = true
			s.mu.Unlock()
			s.save()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "unarchive":
			s.mu.Lock()
			s.Archived = false
			s.mu.Unlock()
			s.save()
			sendJSON(w, 200, map[string]any{"ok": true})
		case "rewind":
			prompt, err := s.RewindTo(num("turn"), boolean("code"), boolean("conversation"))
			if err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"session": s.snapshot(), "prompt": prompt})
		case "summarize":
			if err := s.SummarizeUpTo(r.Context(), num("turn"), str("hint")); err != nil {
				sendJSON(w, 409, map[string]any{"error": err.Error()})
				return true
			}
			sendJSON(w, 200, map[string]any{"session": s.snapshot()})
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
		case "mission":
			var err error
			if str("action") == "stop" {
				s.StopMission()
			} else {
				budget, _ := strconv.ParseFloat(fmt.Sprint(body["budget"]), 64)
				turns, _ := strconv.Atoi(fmt.Sprint(body["maxTurns"]))
				err = s.StartMission(str("goal"), budget, turns)
			}
			sendJSON(w, 200, map[string]any{"ok": err == nil, "error": errString(err), "mission": s.missionSnapshot()})
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
			b, _ := jsonMarshal(body["comments"])
			_ = jsonUnmarshal(b, &items)
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

	// ---------------------------------------------------------------- review
	case path == "/api/changes":
		ch := Changes(dir)
		if ch["isGit"] == false { // a local folder: counted against the copies infuse kept before each first change
			ch = LocalChanges(root, dir)
		}
		sendJSON(w, 200, ch)
		return true
	case path == "/api/pr" && r.Method == http.MethodGet:
		sendJSON(w, 200, PRStatus(dir))
		return true
	case path == "/api/pr":
		var out string
		var err error
		switch str("action") {
		case "create":
			out, err = CreatePR(dir)
		case "merge-auto":
			out, err = ghRun(dir, "pr", "merge", "--auto", "--squash")
		case "failed-logs": // the failing CI run of this branch, for the agent to fix
			out, err = FailedCILogs(dir)
		default:
			err = errors.New("ação desconhecida")
		}
		res := map[string]any{"ok": err == nil, "output": out}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true

	// ---------------------------------------------------------------- files and other apps
	case path == "/api/file" && r.Method == http.MethodGet:
		p := filepath.Join(dir, filepath.FromSlash(q["path"]))
		if rel, err := filepath.Rel(dir, p); err != nil || strings.HasPrefix(rel, "..") {
			sendJSON(w, 400, map[string]any{"error": "fora do projeto"})
			return true
		}
		st, err := os.Stat(p)
		if err != nil {
			sendJSON(w, 404, map[string]any{"error": err.Error()})
			return true
		}
		b, _ := os.ReadFile(p)
		binary := strings.ContainsRune(string(b[:min(len(b), 8000)]), 0)
		if binary || len(b) > 2<<20 {
			sendJSON(w, 200, map[string]any{"path": q["path"], "binary": true, "size": st.Size(), "mtime": st.ModTime().UnixMilli()})
			return true
		}
		sendJSON(w, 200, map[string]any{"path": q["path"], "content": string(b), "mtime": st.ModTime().UnixMilli(), "abs": p,
			"readonly": strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/journal") || strings.HasPrefix(filepath.ToSlash(q["path"]), ".infuse/sessions")})
		return true
	case path == "/api/file" && r.Method == http.MethodPut:
		mtime, _ := body["mtime"].(float64)
		nm, err := WriteUserFile(dir, str("path"), str("content"), int64(mtime), boolean("force"))
		if err == errConflict {
			cur, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(str("path"))))
			sendJSON(w, 409, map[string]any{"conflict": true, "mtime": nm, "content": string(cur)})
			return true
		}
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": true, "mtime": nm})
		return true
	case path == "/api/editors":
		sendJSON(w, 200, Editors())
		return true
	case path == "/api/frameable":
		sendJSON(w, 200, map[string]any{"frameable": frameable(r.Context(), q["url"])})
		return true
	case path == "/api/websearch":
		res, engine, err := WebSearch(r.Context(), q["q"], 12)
		if err != nil {
			sendJSON(w, 200, map[string]any{"error": err.Error(), "results": []SearchResult{}})
			return true
		}
		sendJSON(w, 200, map[string]any{"results": res, "engine": engine})
		return true
	case path == "/api/open" && str("url") != "":
		// "open in the default browser", asked by the user from the browser pane
		u := str("url")
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			sendJSON(w, 400, map[string]any{"error": "endereço inválido"})
			return true
		}
		sendJSON(w, 200, map[string]any{"ok": openDefaultBrowser(u) == nil})
		return true
	case path == "/api/open":
		p := str("path")
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, filepath.FromSlash(p))
		}
		var err error
		if str("app") == "explorer" {
			err = revealInFileManager(p)
		} else {
			err = OpenInEditor(str("app"), p)
		}
		res := map[string]any{"ok": err == nil}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true

	// ---------------------------------------------------------------- terminal
	case path == "/api/term/open":
		cwd := str("cwd")
		if cwd == "" {
			cwd = dir
		}
		t, err := OpenTerminal(root, cwd, num("cols"), num("rows"))
		if err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		if run := str("run"); run != "" { // e.g. an agent CLI: started by the shell, so it runs in the project folder
			_, _ = t.pty.Write([]byte(run + "\r"))
		}
		sendJSON(w, 200, map[string]any{"id": t.ID, "cwd": t.Cwd})
		return true
	case strings.HasPrefix(path, "/api/term/"):
		parts := strings.SplitN(strings.TrimPrefix(path, "/api/term/"), "/", 2)
		t := getTerminal(parts[0])
		if t == nil || len(parts) < 2 {
			sendJSON(w, 404, map[string]any{"error": "terminal encerrado"})
			return true
		}
		switch parts[1] {
		case "stream":
			streamTerminal(w, r, t)
		case "input":
			_, _ = t.pty.Write([]byte(str("data")))
			sendJSON(w, 200, map[string]any{"ok": true})
		case "resize":
			_ = t.pty.Resize(num("cols"), num("rows"))
			sendJSON(w, 200, map[string]any{"ok": true})
		case "close":
			CloseTerminal(t.ID)
			sendJSON(w, 200, map[string]any{"ok": true})
		default:
			return false
		}
		return true

	// ---------------------------------------------------------------- dev servers
	case path == "/api/devservers" && r.Method == http.MethodGet:
		sendJSON(w, 200, DevServers(root))
		return true
	case path == "/api/devservers":
		var err error
		switch str("action") {
		case "approve": // the user approved this exact command in the window
			for _, c := range launchConfigs(root) {
				if c.Name == str("name") {
					err = approveLaunch(root, c)
				}
			}
			if err == nil {
				err = StartDevServer(root, str("name"))
			}
		case "start":
			err = StartDevServer(root, str("name"))
			if na, ok := err.(errLaunchNeedsApproval); ok {
				sendJSON(w, 200, map[string]any{"ok": false, "needsApproval": true, "command": na.Command, "error": na.Error()})
				return true
			}
		case "stop":
			StopDevServer(root, str("name"))
		case "stopall":
			for _, c := range launchConfigs(root) {
				StopDevServer(root, c.Name)
			}
		}
		res := map[string]any{"ok": err == nil, "servers": DevServers(root)}
		if err != nil {
			res["error"] = err.Error()
		}
		sendJSON(w, 200, res)
		return true

	// ---------------------------------------------------------------- scheduled tasks
	case path == "/api/tasks" && r.Method == http.MethodGet:
		sendJSON(w, 200, ListTasks())
		return true
	case path == "/api/tasks" && r.Method == http.MethodPut:
		var t ScheduledTask
		b, _ := jsonMarshal(body)
		_ = jsonUnmarshal(b, &t)
		if old := str("previousName"); old != "" && taskSlug(old) != taskSlug(t.Name) {
			_ = DeleteTask(old)
		}
		saved, err := SaveTask(t)
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		sendJSON(w, 200, saved)
		return true
	case path == "/api/tasks/run":
		id, err := RunTask(str("name"))
		if err != nil {
			sendJSON(w, 400, map[string]any{"error": err.Error()})
			return true
		}
		t, _ := LoadTask(str("name"))
		sendJSON(w, 200, map[string]any{"ok": true, "session": id, "root": t.Root})
		return true
	case path == "/api/tasks/delete":
		_ = DeleteTask(str("name"))
		sendJSON(w, 200, map[string]any{"ok": true})
		return true

	// ---------------------------------------------------------------- custom commands and instructions
	case path == "/api/commands/save":
		name := taskSlug(str("name"))
		if name == "" {
			sendJSON(w, 400, map[string]any{"error": "nome inválido"})
			return true
		}
		base := filepath.Join(root, infuseDirName, "commands")
		if str("scope") == "global" {
			base = filepath.Join(InfuseHome(), "commands")
		}
		_ = os.MkdirAll(base, 0o755)
		content := str("prompt")
		if d := strings.TrimSpace(str("description")); d != "" {
			content = "---\ndescription: " + d + "\n---\n" + content
		}
		if err := os.WriteFile(filepath.Join(base, name+".md"), []byte(content), 0o644); err != nil {
			sendJSON(w, 500, map[string]any{"error": err.Error()})
			return true
		}
		if old := taskSlug(str("previousName")); old != "" && old != name {
			_ = os.Remove(filepath.Join(base, old+".md"))
		}
		sendJSON(w, 200, listCommands(root))
		return true
	case path == "/api/commands/delete":
		for _, base := range []string{filepath.Join(root, infuseDirName, "commands"), filepath.Join(InfuseHome(), "commands")} {
			_ = os.Remove(filepath.Join(base, taskSlug(str("name"))+".md"))
		}
		sendJSON(w, 200, listCommands(root))
		return true
	case path == "/api/projectmode":
		sendJSON(w, 200, map[string]any{"mode": projectMode(root)})
		return true
	case path == "/api/settings/full":
		// settings without secrets plus what the settings screen needs
		s := LoadSettings()
		s.APIKey = ""
		sendJSON(w, 200, map[string]any{"settings": s, "version": version, "shell": GetStatus()})
		return true
	}
	return false
}

func jsonMarshal(v any) ([]byte, error) { return jsonMarshalImpl(v) }

// rememberProjectMode: the mode chosen in the selector is remembered per folder for new sessions.
func rememberProjectMode(root, mode string) {
	if mode == "" {
		return
	}
	list := loadProjects()
	for i := range list {
		if list[i].Root == root {
			list[i].Mode = mode
		}
	}
	_ = saveProjects(list)
}

func projectMode(root string) string {
	for _, p := range loadProjects() {
		if p.Root == root {
			return p.Mode
		}
	}
	return ""
}

var _ = strconv.Itoa
