package agent

// The package's public surface for the API and the composition root. Internals keep their short names.

import (
	"context"
	"infuse/internal/core"
	"infuse/internal/workspace"
	"os"
	"path/filepath"
)

type SessionHit = sessionHit

func NewSession(root string) *Session                 { return newSession(root) }
func LoadSession(root, id string) (*Session, error)   { return loadSession(root, id) }
func ListSessions(root string) []SessionData          { return listSessions(root) }
func SearchSessions(q string, max int) []SessionHit   { return searchSessions(q, max) }
func ResumeInterruptedSessions()                      { resumeInterruptedSessions() }
func SessionsDir(root string) string                  { return sessionsDir(root) }
func ToolSpecs() []core.ToolSpec                      { return toolSpecs() }
func StopAllProcs()                                   { stopAllProcs() }
func LiveSessions(root string) []workspace.LiveSession { return liveSessions(root) }

// RemoveSession drops a session from the live registry and deletes its document and checkpoints.
func RemoveSession(root, id string) {
	sessionsMu.Lock()
	delete(sessions, id)
	sessionsMu.Unlock()
	_ = os.Remove(filepath.Join(sessionsDir(root), id+".json"))
	_ = os.RemoveAll(filepath.Join(root, core.InfuseDirName, "checkpoints", id))
}

func LoadDocs(root string) []PinnedDoc { return loadDocs(root) }
func UnpinDoc(root, url string) error  { return unpinDoc(root, url) }
func DocsPrompt(root string) string    { return docsPrompt(root) }

func AddHook(root, scope, event, matcher, command string) error {
	return addHook(root, scope, event, matcher, command)
}
func RemoveHook(root, key string) error { return removeHook(root, key) }

func (s *Session) Save()                         { s.save() }
func (s *Session) Snapshot() map[string]any      { return s.snapshot() }
func (s *Session) Emit(t string, data any)       { s.emit(t, data) }
func (s *Session) Subscribe() (chan Event, func()) { return s.subscribe() }
func (s *Session) UILang() string                { return s.uiLang() }
func (s *Session) CopyMessages() []Message       { return s.copyMessages() }
func (s *Session) ProcList() []map[string]any    { return s.procList() }
func (s *Session) StopProc(id string) error      { return s.stopProc(id) }
func (s *Session) MissionSnapshot() *Mission     { return s.missionSnapshot() }
func (s *Session) LiftBan(i int, off bool) []Ban { return s.liftBan(i, off) }
func (s *Session) EditMemory(scope string, f func([]MemoryItem) []MemoryItem) error {
	return s.editMemory(scope, f)
}

// ProcOutput returns the stored output of a background process.
func (s *Session) ProcOutput(id string) (string, bool) {
	p := s.proc(id)
	if p == nil {
		return "", false
	}
	text, _ := p.out.since(0)
	return text, true
}

// AllowAlways marks tools the user allowed for this session (scheduled tasks carry their own list).
func (s *Session) AllowAlways(tools []string) {
	s.Mu.Lock()
	for _, tool := range tools {
		s.allowAlways[tool] = true
	}
	s.Mu.Unlock()
}

func liveSessions(root string) []workspace.LiveSession {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	var out []workspace.LiveSession
	for _, s := range sessions {
		s.Mu.Lock()
		if s.Root == root {
			cp := map[int][]core.CheckpointFile{}
			for t, c := range s.Checkpoints {
				cp[t] = append([]core.CheckpointFile{}, c...)
			}
			out = append(out, workspace.LiveSession{ID: s.ID, Created: s.Created, Checkpoints: cp})
		}
		s.Mu.Unlock()
	}
	return out
}

var HookEvents = hookEvents

func HookKey(root, scope, event, matcher, command string) string {
	return hookKey(root, scope, event, matcher, command)
}
func ApproveHook(key string) error { return approveHook(key) }

func ParseSkill(text string) (string, string, string) { return parseSkill(text) }

func ProjectMemoryFile(root string) string { return projectMemoryFile(root) }
func GlobalMemoryFile() string             { return globalMemoryFile() }
func LoadMemory(file string) []MemoryItem  { return loadMemory(file) }
func SaveMemory(file string, items []MemoryItem) error { return saveMemory(file, items) }
func NormalizeMemory(it *MemoryItem) error { return normalizeMemory(it) }

// LiveState is the state of a running session (idle when only its file exists).
func LiveState(id string) string {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := sessions[id]; ok {
		return s.State
	}
	return "idle"
}

func OpenDefaultBrowser(u string) error { return openDefaultBrowser(u) }
func Frameable(ctx context.Context, u string) bool { return frameable(ctx, u) }

func ReadIfExists(p string) string { return readIfExists(p) }
func LoadInstructions(root string) string { return loadInstructions(root) }

func (s *Session) CheckpointBeforeWrite(abs string) { s.checkpointBeforeWrite(abs) }

func (s *Session) LookupTool(name string) (Tool, bool) { return s.lookupTool(name) }
func (s *Session) ToolSpecs() []ToolSpec { return s.toolSpecs() }
