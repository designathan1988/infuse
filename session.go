package main

// Session: the canonical state of one conversation with the agent, its state machine, the agent loop,
// permissions, checkpoints, compaction. The UI only reads this through events and the API.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
	Seq  int    `json:"seq"`
}

type permRequest struct {
	ID       string          `json:"id"`
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Category string          `json:"category"`
	reply    chan string
}

type askRequest struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	reply    chan string
}

type CheckpointFile struct {
	Path string `json:"path"` // relative posix
	New  bool   `json:"new"`  // did not exist before the turn
}

type SessionData struct {
	ID          string                   `json:"id"`
	Root        string                   `json:"root"`
	Title       string                   `json:"title"`
	Created     int64                    `json:"created"`
	Updated     int64                    `json:"updated"`
	Model       string                   `json:"model"`
	Messages    []Message                `json:"messages"`
	Turn        int                      `json:"turn"`
	TurnStarts  []int                    `json:"turnStarts"` // message index where each turn begins
	Tasks       []TaskItem               `json:"tasks"`
	Cwd         string                   `json:"cwd"`
	Usage       Usage                    `json:"usage"`
	Cost        float64                  `json:"cost"`
	Checkpoints map[int][]CheckpointFile `json:"checkpoints"`
	Compactions int                      `json:"compactions"`
	Provider    string                   `json:"provider,omitempty"` // connection id; "" = default
	Mode        string                   `json:"mode,omitempty"`     // permission mode; "" = settings
	Effort      string                   `json:"effort,omitempty"`   // low | medium | high | max
	Archived    bool                     `json:"archived,omitempty"`
	Worktree    string                   `json:"worktree,omitempty"` // absolute path when the session has its own worktree
	Branch      string                   `json:"branch,omitempty"`
	Plan        string                   `json:"plan,omitempty"`      // the latest plan proposed in plan mode
	LastInput   int                      `json:"lastInput,omitempty"` // context size of the latest request
	Memory      []MemoryItem             `json:"memory,omitempty"`    // structured memory of this session only
	Pinned      bool                     `json:"pinned,omitempty"`    // shown at the top of the sidebar
}

type Session struct {
	SessionData
	State string `json:"state"` // idle | thinking | tool | waiting_permission | waiting_user | compacting | error

	mu          sync.Mutex
	subs        map[chan Event]struct{}
	seq         int
	cancel      context.CancelFunc
	pendingPerm *permRequest
	pendingAsk  *askRequest
	readFiles   map[string]bool
	allowAlways map[string]bool
	lastError   string
	mapTimer    *time.Timer
	settings    Settings
}

var (
	sessions   = map[string]*Session{}
	sessionsMu sync.Mutex
)

func sessionsDir(root string) string { return filepath.Join(root, infuseDirName, "sessions") }

func newSession(root string) *Session {
	s := &Session{
		SessionData: SessionData{
			ID: fmt.Sprintf("%d", time.Now().UnixNano()/1e6), Root: root, Created: time.Now().UnixMilli(), Updated: time.Now().UnixMilli(),
			Cwd: root, Checkpoints: map[int][]CheckpointFile{}, Title: "Nova sessão",
		},
		State: "idle", subs: map[chan Event]struct{}{}, readFiles: map[string]bool{}, allowAlways: map[string]bool{},
	}
	s.settings = EffectiveSettings(root)
	s.Model = s.settings.Model
	sessionsMu.Lock()
	sessions[s.ID] = s
	sessionsMu.Unlock()
	s.save()
	RememberProject(root)
	return s
}

func loadSession(root, id string) (*Session, error) {
	sessionsMu.Lock()
	if s, ok := sessions[id]; ok {
		sessionsMu.Unlock()
		return s, nil
	}
	sessionsMu.Unlock()
	var d SessionData
	if err := readJSON(filepath.Join(sessionsDir(root), id+".json"), &d); err != nil {
		return nil, err
	}
	if d.Checkpoints == nil {
		d.Checkpoints = map[int][]CheckpointFile{}
	}
	s := &Session{SessionData: d, State: "idle", subs: map[chan Event]struct{}{}, readFiles: map[string]bool{}, allowAlways: map[string]bool{}}
	s.settings = EffectiveSettings(root)
	if s.Model == "" {
		s.Model = s.settings.Model
	}
	if s.Cwd == "" {
		s.Cwd = root
	}
	sessionsMu.Lock()
	sessions[id] = s
	sessionsMu.Unlock()
	return s, nil
}

func listSessions(root string) []SessionData {
	entries, _ := os.ReadDir(sessionsDir(root))
	var out []SessionData
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		var d SessionData
		if readJSON(filepath.Join(sessionsDir(root), e.Name()), &d) == nil {
			d.Messages = nil
			d.Checkpoints = nil
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	if out == nil {
		out = []SessionData{}
	}
	return out
}

func (s *Session) save() {
	s.Updated = time.Now().UnixMilli()
	_ = writeJSON(filepath.Join(sessionsDir(s.Root), s.ID+".json"), s.SessionData)
}

// ---------------------------------------------------------------- events

func (s *Session) subscribe() (chan Event, func()) {
	ch := make(chan Event, 256)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}

func (s *Session) emit(t string, data any) {
	s.mu.Lock()
	s.seq++
	ev := Event{Type: t, Data: data, Seq: s.seq}
	for ch := range s.subs {
		select {
		case ch <- ev:
		default:
			// a subscriber that fell behind is disconnected rather than silently skipped: its client
			// reconnects and receives a fresh snapshot (pending permission and question included)
			close(ch)
			delete(s.subs, ch)
		}
	}
	s.mu.Unlock()
}

func (s *Session) setState(st string) {
	s.mu.Lock()
	s.State = st
	s.mu.Unlock()
	s.emit("state", map[string]any{"state": st})
}

func (s *Session) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{
		"id": s.ID, "root": s.Root, "title": s.Title, "model": s.Model, "state": s.State, "turn": s.Turn,
		"messages": s.Messages, "tasks": s.Tasks, "cwd": s.Cwd, "usage": s.Usage, "cost": s.Cost,
		"turnStarts": s.TurnStarts, "checkpoints": s.Checkpoints, "pendingPermission": s.pendingPerm,
		"pendingQuestion": s.pendingAsk, "error": s.lastError, "compactions": s.Compactions, "mode": s.mode(),
		"provider": s.providerID(), "effort": s.effort(), "archived": s.Archived, "worktree": s.Worktree, "branch": s.Branch,
		"workRoot": s.workRoot(), "plan": s.Plan, "created": s.Created, "updated": s.Updated, "lastInput": s.LastInput,
	}
}

// mode, providerID, effort: the session's own choice, else the settings default.
func (s *Session) mode() string {
	if s.Mode != "" {
		return s.Mode
	}
	if s.settings.Mode == "" {
		return "default"
	}
	return s.settings.Mode
}

func (s *Session) providerID() string {
	if s.Provider != "" {
		return s.Provider
	}
	return s.settings.Provider
}

func (s *Session) effort() string {
	if s.Effort != "" {
		return s.Effort
	}
	return s.settings.Effort
}

// fastModel is the settings' fast model when the session's provider serves it, else the session's model
// (a fast model from another provider would fail on this connection).
func (s *Session) fastModel() string {
	fm := s.settings.FastModel
	if fm == "" {
		return s.Model
	}
	c := s.settings.Connections[s.providerID()]
	for _, m := range c.Models {
		if m == fm {
			return fm
		}
	}
	if s.providerID() == s.settings.Provider && len(c.Models) == 0 {
		return fm
	}
	return s.Model
}

// workRoot is where the agent's tools operate: the session's worktree, or the project.
func (s *Session) workRoot() string {
	if s.Worktree != "" {
		return s.Worktree
	}
	return s.Root
}

// thinkingBudget maps the effort level to a reasoning budget (0 = off).
func thinkingBudget(effort string, fallback int) int {
	switch effort {
	case "low":
		return 0
	case "medium":
		return 4000
	case "high":
		return 12000
	case "max":
		return 32000
	}
	return fallback
}

// ---------------------------------------------------------------- helpers used by tools

func (s *Session) isProtected(abs string) bool {
	_, ok := newGuard(s.Root, s.settings.ProtectedPaths).protects(s.Root, abs)
	return ok
}

func (s *Session) markRead(abs string) { s.mu.Lock(); s.readFiles[abs] = true; s.mu.Unlock() }
func (s *Session) wasRead(abs string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readFiles[abs]
}
func (s *Session) setCwd(abs string) { s.mu.Lock(); s.Cwd = abs; s.mu.Unlock(); s.emit("cwd", abs) }
func (s *Session) setTasks(t []TaskItem) {
	s.mu.Lock()
	s.Tasks = t
	s.mu.Unlock()
	s.emit("tasks", t)
	s.save()
}

func (s *Session) scheduleMapRefresh() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mapTimer != nil {
		s.mapTimer.Stop()
	}
	root := s.Root
	s.mapTimer = time.AfterFunc(400*time.Millisecond, func() { WriteMap(root, BuildMap(root)); s.emit("map", nil) })
}

func (s *Session) checkpointBeforeWrite(abs string) {
	rel := relTo(s.workRoot(), abs) // relative to where the tools work: a worktree's files stay inside it
	s.mu.Lock()
	for _, c := range s.Checkpoints[s.Turn] {
		if c.Path == rel {
			s.mu.Unlock()
			return
		}
	}
	cf := CheckpointFile{Path: rel, New: !exists(abs)}
	if !cf.New {
		dst := filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(s.Turn), filepath.FromSlash(rel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		if b, err := os.ReadFile(abs); err == nil {
			_ = os.WriteFile(dst, b, 0o644)
		}
	}
	s.Checkpoints[s.Turn] = append(s.Checkpoints[s.Turn], cf)
	s.mu.Unlock()
	s.emit("checkpoint", map[string]any{"turn": s.Turn, "path": rel})
}

// Revert restores files to how they were before turn `toTurn` and truncates the conversation there.
func (s *Session) Revert(toTurn int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.State != "idle" && s.State != "error" {
		return errors.New("pare o agente antes de reverter")
	}
	for t := s.Turn; t >= toTurn; t-- {
		files := s.Checkpoints[t]
		for i := len(files) - 1; i >= 0; i-- {
			c := files[i]
			abs := filepath.Join(s.workRoot(), filepath.FromSlash(c.Path))
			if c.New {
				_ = os.Remove(abs)
				continue
			}
			src := filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t), filepath.FromSlash(c.Path))
			if b, err := os.ReadFile(src); err == nil {
				_ = os.MkdirAll(filepath.Dir(abs), 0o755)
				_ = os.WriteFile(abs, b, 0o644)
			}
		}
		_ = os.RemoveAll(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t)))
		delete(s.Checkpoints, t)
	}
	if toTurn-1 < len(s.TurnStarts) && toTurn >= 1 {
		s.Messages = s.Messages[:s.TurnStarts[toTurn-1]]
		s.TurnStarts = s.TurnStarts[:toTurn-1]
	}
	s.Turn = toTurn - 1
	Journal(s.Root, JournalEntry{Kind: "revert", Detail: fmt.Sprintf("sessão %s revertida ao turno %d", s.ID, toTurn)})
	s.save()
	s.scheduleMapRefreshLocked()
	return nil
}

func (s *Session) scheduleMapRefreshLocked() {
	root := s.Root
	time.AfterFunc(200*time.Millisecond, func() { WriteMap(root, BuildMap(root)); s.emit("map", nil); s.emit("session", s.snapshot()) })
}

// ---------------------------------------------------------------- permissions

func (s *Session) permissionFor(tool Tool, input json.RawMessage) string {
	name := tool.Spec().Name
	cat := tool.Category()
	switch s.mode() {
	case "bypass":
		return "allow"
	case "plan":
		if cat == "write" || cat == "exec" {
			return "deny"
		}
	case "acceptEdits":
		if cat == "write" || cat == "memory" {
			return "allow"
		}
	}
	if cat == "read" || cat == "ui" {
		return "allow"
	}
	s.mu.Lock()
	always := s.allowAlways[name] || s.allowAlways[name+":"+bashPrefix(name, input)]
	s.mu.Unlock()
	if always {
		return "allow"
	}
	if v, ok := s.settings.Permissions[name]; ok {
		return v
	}
	return "ask"
}

func bashPrefix(tool string, input json.RawMessage) string {
	if tool != "Bash" {
		return ""
	}
	var in struct{ Command string }
	_ = json.Unmarshal(input, &in)
	f := strings.Fields(in.Command)
	if len(f) == 0 {
		return ""
	}
	if len(f) > 1 && (f[0] == "git" || f[0] == "npm" || f[0] == "go" || f[0] == "docker" || f[0] == "pnpm" || f[0] == "yarn") {
		return f[0] + " " + f[1]
	}
	return f[0]
}

func (s *Session) askPermission(ctx context.Context, tool Tool, input json.RawMessage) bool {
	req := &permRequest{ID: fmt.Sprintf("p%d", time.Now().UnixNano()), Tool: tool.Spec().Name, Input: input, Category: tool.Category(), reply: make(chan string, 1)}
	s.mu.Lock()
	s.pendingPerm = req
	s.mu.Unlock()
	s.setState("waiting_permission")
	s.emit("permission", req)
	defer func() { s.mu.Lock(); s.pendingPerm = nil; s.mu.Unlock() }()
	select {
	case d := <-req.reply:
		if d == "always" {
			s.mu.Lock()
			s.allowAlways[req.Tool] = true
			if p := bashPrefix(req.Tool, input); p != "" {
				s.allowAlways[req.Tool+":"+p] = true
			}
			s.mu.Unlock()
		}
		return d == "allow" || d == "always"
	case <-ctx.Done():
		return false
	}
}

func (s *Session) ResolvePermission(id, decision string) bool {
	s.mu.Lock()
	req := s.pendingPerm
	s.mu.Unlock()
	if req == nil || req.ID != id {
		return false
	}
	req.reply <- decision
	return true
}

func (s *Session) askUser(ctx context.Context, question string, options []string) (string, bool) {
	req := &askRequest{ID: fmt.Sprintf("q%d", time.Now().UnixNano()), Question: question, Options: options, reply: make(chan string, 1)}
	s.mu.Lock()
	s.pendingAsk = req
	s.mu.Unlock()
	s.setState("waiting_user")
	s.emit("question", req)
	defer func() { s.mu.Lock(); s.pendingAsk = nil; s.mu.Unlock() }()
	select {
	case a := <-req.reply:
		return a, true
	case <-ctx.Done():
		return "", false
	}
}

func (s *Session) Answer(id, answer string) bool {
	s.mu.Lock()
	req := s.pendingAsk
	s.mu.Unlock()
	if req == nil || req.ID != id {
		return false
	}
	req.reply <- answer
	return true
}

// ---------------------------------------------------------------- the loop

// InlineImage is an image pasted into the prompt box.
type InlineImage struct {
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}

// userBlocks builds the user message: the text, then each attached file (images as image blocks, other
// files as their content, with the path), then pasted images.
func userBlocks(text string, atts []Attachment, imgs []InlineImage) []Block {
	blocks := []Block{{Type: "text", Text: text}}
	for _, a := range atts {
		b, err := os.ReadFile(a.Path)
		if err != nil {
			continue
		}
		if mt, ok := imageExt[strings.ToLower(filepath.Ext(a.Path))]; ok {
			blocks = append(blocks, Block{Type: "image", MediaType: mt, Data: base64.StdEncoding.EncodeToString(b)})
			continue
		}
		if len(b) > 400_000 {
			b = b[:400_000]
		}
		blocks = append(blocks, Block{Type: "text", Text: fmt.Sprintf("<arquivo anexado path=%q>\n%s\n</arquivo>", a.Path, string(b))})
	}
	for _, im := range imgs {
		if im.Data != "" {
			blocks = append(blocks, Block{Type: "image", MediaType: im.MediaType, Data: im.Data})
		}
	}
	return blocks
}

func (s *Session) Send(text string, atts []Attachment, imgs []InlineImage) error {
	s.mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.mu.Unlock()
		return errors.New("o agente está ocupado")
	}
	s.settings = EffectiveSettings(s.Root)
	if s.Model == "" {
		s.Model = s.settings.Model
	}
	s.lastError = ""
	s.Turn++
	s.TurnStarts = append(s.TurnStarts, len(s.Messages))
	s.Messages = append(s.Messages, Message{Role: "user", Content: userBlocks(text, atts, imgs), TS: time.Now().UnixMilli()})
	if s.Turn == 1 && (s.Title == "" || s.Title == "Nova sessão") {
		s.Title = truncate(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0], 60)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()
	s.emit("turn", map[string]any{"turn": s.Turn, "text": text, "title": s.Title})
	s.save()
	go s.run(ctx)
	return nil
}

func (s *Session) Stop() {
	s.mu.Lock()
	c := s.cancel
	s.mu.Unlock()
	if c != nil {
		c()
	}
}

func (s *Session) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.mu.Lock()
			s.lastError = fmt.Sprint(r)
			s.mu.Unlock()
			s.setState("error")
		}
		s.save()
	}()
	if s.providerID() == "" {
		s.fail(errors.New("nenhum provedor conectado: abra Configurações > Provedores e conecte um"))
		return
	}
	provider, err := providerFor(s.settings, s.providerID())
	if err != nil {
		s.fail(err)
		return
	}
	rules := LoadRules(s.Root)
	var breaker loopBreaker // stops a turn that keeps repeating the same failure
	var tripped error
	for iter := 0; iter < 200; iter++ {
		if ctx.Err() != nil {
			s.finish("cancelado")
			return
		}
		if s.needsCompaction() {
			if err := s.compact(ctx, provider); err != nil {
				s.fail(fmt.Errorf("compactação: %w", err))
				return
			}
		}
		s.setState("thinking")
		budget := thinkingBudget(s.effort(), s.settings.ThinkingBudget)
		maxTok := s.settings.MaxTokens
		if budget > 0 && maxTok <= budget+1024 {
			maxTok = budget + 8000
		}
		req := ModelRequest{Model: s.Model, System: buildSystemPrompt(s, rules), Messages: s.copyMessages(), Tools: toolSpecs(), MaxTokens: maxTok, Thinking: budget}
		started := time.Now()
		var thinkFrom, thinkTo time.Time // how long the model spent in its reasoning, from the stream
		res, err := provider.Stream(ctx, req, func(ev StreamEvent) {
			if ev.Kind == "thinking" && thinkFrom.IsZero() {
				thinkFrom = time.Now()
			} else if ev.Kind != "thinking" && !thinkFrom.IsZero() && thinkTo.IsZero() {
				thinkTo = time.Now()
			}
			s.emit("stream", ev)
		})
		if err != nil {
			if ctx.Err() != nil {
				s.finish("cancelado")
				return
			}
			s.fail(err)
			return
		}
		if !thinkFrom.IsZero() {
			if thinkTo.IsZero() {
				thinkTo = time.Now()
			}
			for i := range res.Content {
				if res.Content[i].Type == "thinking" {
					res.Content[i].Ms = thinkTo.Sub(thinkFrom).Milliseconds()
				}
			}
		}
		reply := Message{Role: "assistant", Content: res.Content, TS: time.Now().UnixMilli(), Ms: time.Since(started).Milliseconds(), Model: s.Model, Tokens: res.Usage.Output}
		s.mu.Lock()
		s.Messages = append(s.Messages, reply)
		s.Usage.Input += res.Usage.Input
		s.Usage.Output += res.Usage.Output
		s.Usage.CacheRead += res.Usage.CacheRead
		s.Usage.CacheWrite += res.Usage.CacheWrite
		cost := s.settings.costOf(s.Model, res.Usage)
		s.Cost += cost
		s.LastInput = res.Usage.Input + res.Usage.CacheRead + res.Usage.CacheWrite
		if s.mode() == "plan" && res.StopReason != "tool_use" {
			for _, b := range res.Content {
				if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
					s.Plan = b.Text
				}
			}
		}
		s.mu.Unlock()
		recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.Model, Input: res.Usage.Input, Output: res.Usage.Output, Cache: res.Usage.CacheRead + res.Usage.CacheWrite, Cost: cost})
		s.emit("message", map[string]any{"message": reply, "usage": res.Usage, "totalUsage": s.Usage, "cost": s.Cost, "lastInput": res.Usage.Input + res.Usage.CacheRead + res.Usage.CacheWrite})
		s.save()
		if lim := s.settings.DailyLimitUSD; lim > 0 { // the day's total across every session, from the ledger
			if spent := todaySpend(); spent > lim {
				s.fail(fmt.Errorf("limite de gasto diário atingido: US$ %.2f de US$ %.2f (Configurações > Modelos e custos)", spent, lim))
				return
			}
		}
		if res.StopReason != "tool_use" {
			s.finish("")
			return
		}
		var results []Block
		for _, b := range res.Content {
			if b.Type != "tool_use" {
				continue
			}
			toolStart := time.Now()
			out, isErr := s.runTool(ctx, rules, b)
			out = redactSecrets(out) // credentials never reach the model, the transcript or the UI
			ms := time.Since(toolStart).Milliseconds()
			results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr, Ms: ms})
			s.emit("tool_result", map[string]any{"id": b.ID, "name": b.Name, "output": out, "isError": isErr, "ms": ms})
			if err := breaker.record(b.Name, b.Input, isErr); err != nil {
				tripped = err
			}
			if ctx.Err() != nil || tripped != nil {
				break
			}
		}
		if ctx.Err() != nil || tripped != nil {
			s.mu.Lock()
			// keep the conversation consistent: every tool_use gets a result
			why := "cancelado pelo usuário"
			if tripped != nil {
				why = tripped.Error()
			}
			for _, b := range res.Content {
				if b.Type == "tool_use" && !hasResult(results, b.ID) {
					results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: why, IsError: true})
				}
			}
			s.Messages = append(s.Messages, Message{Role: "user", Content: results, TS: time.Now().UnixMilli()})
			s.mu.Unlock()
			if tripped != nil {
				Lesson(s.Root, "loop-errático", tripped.Error())
				s.fail(tripped)
				return
			}
			s.finish("cancelado")
			return
		}
		s.mu.Lock()
		s.Messages = append(s.Messages, Message{Role: "user", Content: results, TS: time.Now().UnixMilli()})
		s.mu.Unlock()
		s.save()
	}
	s.fail(errors.New("limite de iterações do turno atingido"))
}

func hasResult(results []Block, id string) bool {
	for _, r := range results {
		if r.ToolUseID == id {
			return true
		}
	}
	return false
}

// readsOutside reports whether a read tool's path (path / file_path) points outside the session's folder.
func (s *Session) readsOutside(input json.RawMessage) bool {
	var in struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)
	p := in.Path
	if p == "" {
		p = in.FilePath
	}
	if p == "" {
		return false
	}
	p = FromMsysPath(p)
	s.mu.Lock()
	cwd, root := s.Cwd, s.workRoot()
	s.mu.Unlock()
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return !within(root, p)
}

func (s *Session) runTool(ctx context.Context, rules *RuleSet, b Block) (string, bool) {
	tool, ok := toolRegistry[b.Name]
	if !ok {
		return "ferramenta desconhecida: " + b.Name, true
	}
	decision := s.permissionFor(tool, b.Input)
	// reading outside the session's folder (~/.ssh, browser profiles…) is never silent: it asks,
	// unless the user chose to accept everything
	if decision == "allow" && tool.Category() == "read" && s.mode() != "bypass" && s.readsOutside(b.Input) {
		decision = "ask"
	}
	switch decision {
	case "deny":
		if s.mode() == "plan" {
			return "modo plano: escrita e execução estão desativadas; proponha o plano ao usuário", true
		}
		return "permissão negada pelas configurações do usuário", true
	case "ask":
		if !s.askPermission(ctx, tool, b.Input) {
			return "o usuário negou esta ação", true
		}
	}
	s.setState("tool")
	s.emit("tool_run", map[string]any{"id": b.ID, "name": b.Name})
	s.mu.Lock()
	tc := &ToolContext{S: s, Root: s.workRoot(), Cwd: s.Cwd, Rules: rules}
	s.mu.Unlock()
	return tool.Run(ctx, tc, b.Input)
}

func (s *Session) copyMessages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.Messages))
	copy(out, s.Messages)
	return out
}

func (s *Session) finish(reason string) {
	s.mu.Lock()
	s.cancel = nil
	s.mu.Unlock()
	s.setState("idle")
	s.emit("done", map[string]any{"reason": reason, "cost": s.Cost, "usage": s.Usage})
	s.save()
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	s.lastError = err.Error()
	s.cancel = nil
	s.mu.Unlock()
	s.emit("error", err.Error())
	s.setState("error")
	s.save()
}

// ---------------------------------------------------------------- compaction

func (s *Session) needsCompaction() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Estimate: 4 chars per token over the serialized messages.
	b, _ := json.Marshal(s.Messages)
	return len(b)/4 > int(float64(s.settings.ContextLimit)*0.75)
}

func (s *Session) compact(ctx context.Context, p Provider) error {
	s.setState("compacting")
	msgs := s.copyMessages()
	if len(msgs) < 4 {
		return nil
	}
	keep := msgs[len(msgs)-2:] // the latest exchange stays verbatim
	older := msgs[:len(msgs)-2]
	older = append(older, Message{Role: "user", Content: []Block{{Type: "text", Text: "Resuma a conversa acima para continuar o trabalho sem perder nada essencial: objetivo, decisões tomadas, arquivos criados ou alterados (com caminhos), o que foi tentado e rejeitado, erros encontrados, estado atual e próximos passos. Seja específico e conciso."}}})
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), Messages: older, MaxTokens: 4000}, func(StreamEvent) {})
	if err != nil {
		return err
	}
	summary := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			summary += b.Text
		}
	}
	if v := verbatimRequests(older[:len(older)-1]); v != "" {
		summary += "\n\n## Pedidos do usuário, literais\n" + v
	}
	s.mu.Lock()
	s.Messages = append([]Message{{Role: "user", Content: []Block{{Type: "text", Text: "[Contexto compactado — resumo do trabalho até aqui]\n\n" + summary}}}, {Role: "assistant", Content: []Block{{Type: "text", Text: "Entendido, continuo a partir desse estado."}}}}, keep...)
	s.TurnStarts = []int{0}
	s.Compactions++
	s.Cost += s.settings.costOf(s.fastModel(), res.Usage)
	s.mu.Unlock()
	s.emit("compacted", map[string]any{"summary": summary})
	return nil
}

// verbatimRequests returns what the user typed in msgs, word for word, newest kept whole when the
// budget runs out: a summary may lose a nuance; these islands keep the user's own wording.
func verbatimRequests(msgs []Message) string {
	var reqs []string
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, b := range m.Content {
			if b.Type == "text" && strings.HasPrefix(b.Text, "[Contexto compactado") { // islands kept by an earlier compaction
				const head = "## Pedidos do usuário, literais\n"
				if k := strings.Index(b.Text, head); k >= 0 {
					for _, chunk := range strings.Split(b.Text[k+len(head):], "\n\n") {
						if strings.HasPrefix(chunk, "> ") {
							reqs = append(reqs, strings.ReplaceAll(strings.TrimPrefix(chunk, "> "), "\n> ", "\n"))
						}
					}
				}
				break
			}
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" && !strings.HasPrefix(b.Text, "[") && !strings.HasPrefix(b.Text, "<") {
				reqs = append(reqs, strings.TrimSpace(b.Text))
				break
			}
		}
	}
	const budget = 6000
	var out []string
	used := 0
	for i := len(reqs) - 1; i >= 0; i-- {
		r := reqs[i]
		if used+len(r) > budget {
			if used == 0 {
				r = r[:budget] + " […]"
			} else {
				out = append(out, fmt.Sprintf("(%d pedido(s) mais antigo(s) omitido(s))", i+1))
				break
			}
		}
		used += len(r)
		out = append(out, "> "+strings.ReplaceAll(r, "\n", "\n> "))
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return strings.Join(out, "\n\n")
}

// ---------------------------------------------------------------- rewind variants, summaries, side chat

// RewindTo restores files and/or the conversation to before turn toTurn. It returns the prompt that
// started that turn, so the window can put it back in the prompt box.
func (s *Session) RewindTo(toTurn int, code, conversation bool) (string, error) {
	s.mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.mu.Unlock()
		return "", errors.New("pare o agente antes de voltar")
	}
	if toTurn < 1 || toTurn > s.Turn {
		s.mu.Unlock()
		return "", errors.New("turno inexistente")
	}
	prompt := ""
	if toTurn-1 < len(s.TurnStarts) {
		if i := s.TurnStarts[toTurn-1]; i < len(s.Messages) {
			for _, b := range s.Messages[i].Content {
				if b.Type == "text" {
					prompt = b.Text
					break
				}
			}
		}
	}
	s.mu.Unlock()
	if code && conversation {
		return prompt, s.Revert(toTurn)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if code {
		for t := s.Turn; t >= toTurn; t-- {
			files := s.Checkpoints[t]
			for i := len(files) - 1; i >= 0; i-- {
				c := files[i]
				abs := filepath.Join(s.workRoot(), filepath.FromSlash(c.Path))
				if c.New {
					_ = os.Remove(abs)
					continue
				}
				src := filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t), filepath.FromSlash(c.Path))
				if b, err := os.ReadFile(src); err == nil {
					_ = os.MkdirAll(filepath.Dir(abs), 0o755)
					_ = os.WriteFile(abs, b, 0o644)
				}
			}
			_ = os.RemoveAll(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t)))
			delete(s.Checkpoints, t)
		}
		Journal(s.Root, JournalEntry{Kind: "revert", Detail: fmt.Sprintf("sessão %s: código restaurado a antes do turno %d", s.ID, toTurn)})
	}
	if conversation {
		for t := s.Turn; t >= toTurn; t-- { // later checkpoints no longer belong to any turn
			_ = os.RemoveAll(filepath.Join(s.Root, infuseDirName, "checkpoints", s.ID, fmt.Sprint(t)))
			delete(s.Checkpoints, t)
		}
		if toTurn-1 < len(s.TurnStarts) {
			s.Messages = s.Messages[:s.TurnStarts[toTurn-1]]
			s.TurnStarts = s.TurnStarts[:toTurn-1]
		}
		s.Turn = toTurn - 1
	}
	s.save()
	s.scheduleMapRefreshLocked()
	return prompt, nil
}

// SummarizeUpTo compresses the conversation before turn upTo into a summary; later turns stay verbatim.
func (s *Session) SummarizeUpTo(ctx context.Context, upTo int, hint string) error {
	s.mu.Lock()
	if s.State != "idle" && s.State != "error" {
		s.mu.Unlock()
		return errors.New("pare o agente antes de resumir")
	}
	if upTo < 2 || upTo-1 >= len(s.TurnStarts) {
		s.mu.Unlock()
		return errors.New("nada para resumir antes deste turno")
	}
	cut := s.TurnStarts[upTo-1]
	older := append([]Message{}, s.Messages[:cut]...)
	s.mu.Unlock()
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return err
	}
	ask := "Resuma a conversa acima para continuar o trabalho sem perder nada essencial: objetivo, decisões, arquivos criados ou alterados (com caminhos), o que foi tentado e rejeitado, erros, estado atual e próximos passos."
	if strings.TrimSpace(hint) != "" {
		ask += "\nFoco pedido pelo usuário: " + hint
	}
	older = append(older, Message{Role: "user", Content: []Block{{Type: "text", Text: ask}}})
	model := s.fastModel()
	res, err := p.Stream(ctx, ModelRequest{Model: model, Messages: older, MaxTokens: 4000}, func(StreamEvent) {})
	if err != nil {
		return err
	}
	summary := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			summary += b.Text
		}
	}
	s.mu.Lock()
	head := []Message{{Role: "user", Content: []Block{{Type: "text", Text: "[Conversa resumida]\n\n" + summary}}}, {Role: "assistant", Content: []Block{{Type: "text", Text: "Entendido, continuo a partir desse estado."}}}}
	rest := s.Messages[cut:]
	shift := cut - len(head)
	var starts []int
	starts = append(starts, 0)
	for _, st := range s.TurnStarts[upTo-1:] {
		starts = append(starts, st-shift)
	}
	s.Messages = append(head, rest...)
	s.TurnStarts = starts
	s.Turn = len(starts)
	s.Compactions++
	s.Cost += s.settings.costOf(model, res.Usage)
	s.mu.Unlock()
	s.save()
	s.emit("session", s.snapshot())
	return nil
}

// SideQuestion answers a question about the session without adding anything to its history.
func (s *Session) SideQuestion(ctx context.Context, question string) (string, error) {
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return "", err
	}
	msgs := s.copyMessages()
	// a trailing tool_use without its result would be rejected; cut back to the last complete exchange
	for len(msgs) > 0 && msgs[len(msgs)-1].Role == "assistant" {
		hasTool := false
		for _, b := range msgs[len(msgs)-1].Content {
			if b.Type == "tool_use" {
				hasTool = true
			}
		}
		if !hasTool {
			break
		}
		msgs = msgs[:len(msgs)-1]
	}
	if len(msgs) > 0 && msgs[len(msgs)-1].Role == "user" {
		msgs = append(msgs, Message{Role: "assistant", Content: []Block{{Type: "text", Text: "(continuando)"}}})
	}
	msgs = append(msgs, Message{Role: "user", Content: []Block{{Type: "text", Text: "[Pergunta paralela do usuário — responda direto, sem usar ferramentas; isto não entra no histórico]\n" + question}}})
	res, err := p.Stream(ctx, ModelRequest{Model: s.Model, System: "Você responde perguntas rápidas sobre a sessão de trabalho em andamento.", Messages: msgs, MaxTokens: 2000}, func(StreamEvent) {})
	if err != nil {
		return "", err
	}
	out := ""
	for _, b := range res.Content {
		if b.Type == "text" {
			out += b.Text
		}
	}
	cost := s.settings.costOf(s.Model, res.Usage)
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.Model, Input: res.Usage.Input, Output: res.Usage.Output, Cost: cost})
	return out, nil
}

// ContextBreakdown estimates where the context window goes (4 characters ≈ 1 token).
func (s *Session) ContextBreakdown() map[string]any {
	rules := LoadRules(s.Root)
	sys := len(buildSystemPrompt(s, rules)) / 4
	tb, _ := json.Marshal(toolSpecs())
	tools := len(tb) / 4
	var user, assistant, calls, results, images int
	for _, m := range s.copyMessages() {
		for _, b := range m.Content {
			switch b.Type {
			case "text":
				if m.Role == "user" {
					user += len(b.Text) / 4
				} else {
					assistant += len(b.Text) / 4
				}
			case "thinking":
				assistant += len(b.Thinking) / 4
			case "tool_use":
				calls += (len(b.Input) + len(b.Name)) / 4
			case "tool_result":
				results += len(b.Content) / 4
			case "image":
				images += 1600
			}
		}
	}
	total := sys + tools + user + assistant + calls + results + images
	limit := s.settings.ContextLimit
	return map[string]any{
		"limit": limit, "total": total,
		"categories": []map[string]any{
			{"name": "Instruções do sistema", "tokens": sys}, {"name": "Ferramentas", "tokens": tools},
			{"name": "Suas mensagens", "tokens": user}, {"name": "Respostas", "tokens": assistant},
			{"name": "Chamadas de ferramenta", "tokens": calls}, {"name": "Resultados de ferramenta", "tokens": results},
			{"name": "Imagens", "tokens": images},
		},
	}
}

// SuggestNext guesses the user's likely next message from the end of the conversation, for the gray
// suggestion in the prompt box (Tab accepts). The fast model sees only the recent text, no tools.
func (s *Session) SuggestNext(ctx context.Context) (string, error) {
	p, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return "", err
	}
	msgs := s.copyMessages()
	var lines []string
	for i := len(msgs) - 1; i >= 0 && len(lines) < 6; i-- {
		if t := strings.TrimSpace(msgText(msgs[i])); t != "" && !strings.HasPrefix(t, "[") {
			who := "Agente"
			if msgs[i].Role == "user" {
				who = "Usuário"
			}
			if len(t) > 1200 {
				t = t[:1200] + "…"
			}
			lines = append([]string{who + ": " + t}, lines...)
		}
	}
	if len(lines) == 0 {
		return "", nil
	}
	ask := "Conversa recente entre um usuário e um agente de programação:\n\n" + strings.Join(lines, "\n\n") +
		"\n\nEscreva a próxima mensagem mais provável do usuário, curta (até 12 palavras), no idioma dele. Responda só com a mensagem, sem aspas. Se não houver um próximo passo claro, responda com um traço (-)."
	res, err := p.Stream(ctx, ModelRequest{Model: s.fastModel(), Messages: []Message{{Role: "user", Content: []Block{{Type: "text", Text: ask}}}}, MaxTokens: 60}, func(StreamEvent) {})
	if err != nil {
		return "", err
	}
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.fastModel(), Input: res.Usage.Input, Output: res.Usage.Output, Cost: s.settings.costOf(s.fastModel(), res.Usage)})
	out := strings.Trim(strings.TrimSpace(msgText(Message{Content: res.Content})), "\"'“”")
	if out == "-" || out == "" || strings.Contains(out, "\n") || len([]rune(out)) > 120 {
		return "", nil
	}
	return out, nil
}

// msgText joins the text blocks of a message.
func msgText(m Message) string {
	var b strings.Builder
	for _, c := range m.Content {
		if c.Type == "text" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

type sessionHit struct {
	ID      string `json:"id"`
	Root    string `json:"root"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// searchSessions finds q (case-insensitive) in the titles and messages of every project's sessions.
func searchSessions(q string, max int) []sessionHit {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []sessionHit{}
	if len([]rune(q)) < 2 {
		return out
	}
	for _, p := range loadProjects() {
		entries, _ := os.ReadDir(sessionsDir(p.Root))
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") || len(out) >= max {
				continue
			}
			var d SessionData
			if readJSON(filepath.Join(sessionsDir(p.Root), e.Name()), &d) != nil {
				continue
			}
			hit := ""
			if strings.Contains(strings.ToLower(d.Title), q) {
				hit = d.Title
			}
			for i := len(d.Messages) - 1; i >= 0 && hit == ""; i-- {
				for _, b := range d.Messages[i].Content {
					text := b.Text
					if b.Type == "tool_result" {
						text = b.Content
					}
					low := strings.ToLower(text)
					if k := strings.Index(low, q); k >= 0 {
						hit = snippetAround(text, k, len(q))
						break
					}
				}
			}
			if hit != "" {
				out = append(out, sessionHit{ID: d.ID, Root: d.Root, Title: d.Title, Snippet: hit})
			}
		}
	}
	return out
}

// snippetAround cuts ~40 characters on each side of the match at byte k (rune-safe).
func snippetAround(text string, k, n int) string {
	from, to := max(0, k-40), min(len(text), k+n+40)
	for from > 0 && !utf8.RuneStart(text[from]) {
		from--
	}
	for to < len(text) && !utf8.RuneStart(text[to]) {
		to++
	}
	s := strings.Join(strings.Fields(text[from:to]), " ")
	if from > 0 {
		s = "…" + s
	}
	if to < len(text) {
		s += "…"
	}
	return s
}
