package agent

// The per-turn log: one JSON line per model request and per tool call, in .infuse/turnlog/<session>.jsonl.
// It holds metadata (sizes, timings, usage, the calls and their trimmed input), not the transcript, and
// passes through the same redaction as everything else shown. Readers go by the record's own schema
// version, so older lines stay readable.

import (
	"crypto/sha256"
	"fmt"
	"infuse/internal/core"
	"path/filepath"
	"strings"
	"time"
)

const (
	turnLogSchema   = 1
	turnLogInputMax = 2000
)

// TurnRecord is one line of the log.
type TurnRecord struct {
	V        int          `json:"v"`    // schema version
	Kind     string       `json:"kind"` // model | tool
	Session  string       `json:"session"`
	Turn     int          `json:"turn"`
	TS       int64        `json:"ts"`
	Ms       int64        `json:"ms"`
	Model    string       `json:"model,omitempty"`
	Provider string       `json:"provider,omitempty"`
	Request  *TurnRequest `json:"request,omitempty"`
	Stop     string       `json:"stop,omitempty"`
	Usage    *Usage       `json:"usage,omitempty"`
	Cost     float64      `json:"cost,omitempty"`
	Tool     *TurnTool    `json:"tool,omitempty"`
}

// TurnRequest describes what was sent to the model; the prompt itself is identified by its digest.
type TurnRequest struct {
	SystemBytes  int    `json:"systemBytes"`
	SystemDigest string `json:"systemDigest"`
	Messages     int    `json:"messages"`
	Tools        int    `json:"tools"`
	MaxTokens    int    `json:"maxTokens"`
	Thinking     int    `json:"thinking,omitempty"`
}

// TurnTool is one tool call.
type TurnTool struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Input       string `json:"input"`
	IsError     bool   `json:"isError,omitempty"`
	OutputBytes int    `json:"outputBytes"`
}

func turnLogFile(root, session string) string {
	return filepath.Join(root, infuseDirName, "turnlog", session+".jsonl")
}

func (s *Session) writeTurnRecord(r TurnRecord) {
	r.V, r.Session, r.TS = turnLogSchema, s.ID, time.Now().UnixMilli()
	s.Mu.Lock()
	r.Turn = s.Turn
	s.Mu.Unlock()
	core.AppendLine(turnLogFile(s.Root, s.ID), r)
	exportOTLP(s, r)
}

// logModelCall records one model request.
func (s *Session) logModelCall(req ModelRequest, res ModelResult, started time.Time) {
	sum := sha256.Sum256([]byte(req.System))
	usage := res.Usage
	s.writeTurnRecord(TurnRecord{
		Kind: "model", Ms: time.Since(started).Milliseconds(), Model: req.Model, Provider: s.providerID(), Stop: res.StopReason,
		Usage: &usage, Cost: s.settings.CostOf(req.Model, res.Usage),
		Request: &TurnRequest{SystemBytes: len(req.System), SystemDigest: fmt.Sprintf("%x", sum[:8]), Messages: len(req.Messages), Tools: len(req.Tools), MaxTokens: req.MaxTokens, Thinking: req.Thinking},
	})
}

// logToolCalls records the tool calls of a reply, with their results.
func (s *Session) logToolCalls(res ModelResult, results []Block) {
	byID := map[string]Block{}
	for _, r := range results {
		byID[r.ToolUseID] = r
	}
	for _, b := range toolUses(res.Content) {
		r := byID[b.ID]
		input := s.maskSettingsEnv(redactSecrets(truncate(string(b.Input), turnLogInputMax)))
		s.writeTurnRecord(TurnRecord{Kind: "tool", Ms: r.Ms, Tool: &TurnTool{ID: b.ID, Name: b.Name, Input: input, IsError: r.IsError, OutputBytes: len(r.Content)}})
	}
}

// TurnQuery filters the log; zero values match everything.
type TurnQuery struct {
	Session string
	Turn    int
	Kind    string
	Tool    string
	Limit   int
}

// QueryTurnLog returns the matching records of one session, the newest last.
func QueryTurnLog(root string, q TurnQuery) []map[string]any {
	out := []map[string]any{}
	for _, rec := range core.ReadJSONL(turnLogFile(root, sanitizeLogID(q.Session))) {
		if q.matches(rec) {
			out = append(out, rec)
		}
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[len(out)-q.Limit:]
	}
	return out
}

func (q TurnQuery) matches(rec map[string]any) bool {
	if turn, _ := rec["turn"].(float64); q.Turn > 0 && int(turn) != q.Turn {
		return false
	}
	if kind, _ := rec["kind"].(string); q.Kind != "" && kind != q.Kind {
		return false
	}
	if q.Tool != "" {
		tool, _ := rec["tool"].(map[string]any)
		if name, _ := tool["name"].(string); !strings.EqualFold(name, q.Tool) {
			return false
		}
	}
	return true
}

// sanitizeLogID keeps a session id from a request inside the log's folder.
func sanitizeLogID(id string) string { return filepath.Base(filepath.Clean("/" + id)) }
