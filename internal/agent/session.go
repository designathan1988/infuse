package agent

// Session: the canonical state of one conversation with the agent, its state machine, the agent loop,
// permissions, checkpoints, compaction. The UI only reads this through events and the API.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type permRequest struct {
	ID       string          `json:"id"`
	Tool     string          `json:"tool"`
	Input    json.RawMessage `json:"input"`
	Category string          `json:"category"`
	Reason   string          `json:"reason,omitempty"` // why infuse asks although the mode would allow it (a sentinel)
	reply    chan string
}

type askRequest struct {
	ID       string   `json:"id"`
	Question string   `json:"question"`
	Options  []string `json:"options"`
	reply    chan string
}

type SessionData struct {
	ID          string                   `json:"id"`
	Root        string                   `json:"root"`
	Title       string                   `json:"title"`
	Created     int64                    `json:"created"`
	Updated     int64                    `json:"updated"`
	Model       string                   `json:"model"`
	Messages    []Message                `json:"messages"`
	Agents      []AgentRun               `json:"agents,omitempty"` // every subagent of the session, with its result
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
	Diagnoses   []Diagnosis              `json:"diagnoses,omitempty"` // the diagnoses registered, with their verified evidence
	Working     bool                     `json:"working,omitempty"`   // a turn was in progress when last saved (resumed at startup)
	Worktree    string                   `json:"worktree,omitempty"`  // absolute path when the session has its own worktree
	Branch      string                   `json:"branch,omitempty"`
	Plan        string                   `json:"plan,omitempty"`       // the latest plan proposed in plan mode
	LastInput   int                      `json:"lastInput,omitempty"`  // context size of the latest request
	Memory      []MemoryItem             `json:"memory,omitempty"`     // structured memory of this session only
	Pinned      bool                     `json:"pinned,omitempty"`     // shown at the top of the sidebar
	Mission     *Mission                 `json:"mission,omitempty"`    // the session's mission, running or ended
	PinnedMsgs  []int                    `json:"pinnedMsgs,omitempty"` // messages the user pinned (indices)
}

type Session struct {
	SessionData
	State string `json:"state"` // idle | thinking | tool | waiting_permission | waiting_user | compacting | error

	Mu            sync.Mutex
	subs          map[chan Event]struct{}
	seq           int
	cancel        context.CancelFunc
	pendingPerm   *permRequest
	pendingAsk    *askRequest
	readFiles     map[string]bool
	allowAlways   map[string]bool
	lastError     string
	mapTimer      *time.Timer
	settings      Settings
	inflight      []Block               // results of the current reply's tool calls, before they join the transcript
	pendingImages []Block               // images the agent opened with Read, sent with the next tool results
	askMu         sync.Mutex            // one question to the user at a time (parallel subagents may each need one)
	gate          researchGate          // research gate and report requirement of the current turn
	docs          docLedger             // what the agent read as documentation in this session (docgate.go)
	procs         []*bgProc             // background processes (procs.go)
	cover         map[string]*readCover // lines of each file actually shown to the agent (instructions_gate.go)
	callMsgs      map[string][]Msg      // infuse's messages recorded per tool call (msgs.go)
	procSeq       int
}

var (
	sessions   = map[string]*Session{}
	sessionsMu sync.Mutex
)

func sessionsDir(root string) string { return filepath.Join(root, infuseDirName, "sessions") }

var sessionIDMu sync.Mutex

// newSessionID is the creation time in milliseconds (sessions sort by it), moved forward when that
// millisecond is taken — two sessions created in the same millisecond (a fork) must not share an id.
func newSessionID(root string) string {
	sessionIDMu.Lock()
	defer sessionIDMu.Unlock()
	ms := time.Now().UnixNano() / 1e6
	for {
		id := fmt.Sprintf("%d", ms)
		sessionsMu.Lock()
		_, taken := sessions[id]
		sessionsMu.Unlock()
		if !taken && !exists(filepath.Join(sessionsDir(root), id+".json")) {
			return id
		}
		ms++
	}
}

func newSession(root string) *Session {
	s := &Session{
		SessionData: SessionData{
			ID: newSessionID(root), Root: root, Created: time.Now().UnixMilli(), Updated: time.Now().UnixMilli(),
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
	// Several requests can load the same session at once (opening it fires a few); the first one stored
	// wins, so every caller — the event stream included — holds the same object the agent runs on.
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if existing, ok := sessions[id]; ok {
		return existing, nil
	}
	sessions[id] = s
	return s, nil
}

// sessionListCache keeps the sidebar's summary of each session file, by modification time and size:
// the list is asked for often and a session file carries the whole conversation.
var (
	sessionListCache   = map[string]sessionListEntry{}
	sessionListCacheMu sync.Mutex
)

type sessionListEntry struct {
	mod  time.Time
	size int64
	data SessionData
}

func listSessions(root string) []SessionData {
	entries, _ := os.ReadDir(sessionsDir(root))
	var out []SessionData
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(sessionsDir(root), e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		sessionListCacheMu.Lock()
		c, ok := sessionListCache[p]
		sessionListCacheMu.Unlock()
		if ok && c.mod.Equal(info.ModTime()) && c.size == info.Size() {
			out = append(out, c.data)
			continue
		}
		var d SessionData
		if readJSON(p, &d) == nil {
			d.Messages = nil
			d.Checkpoints = nil
			d.Memory = nil
			sessionListCacheMu.Lock()
			sessionListCache[p] = sessionListEntry{mod: info.ModTime(), size: info.Size(), data: d}
			sessionListCacheMu.Unlock()
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
