package main

// Sentinels: deterministic checks on the agent's actions, no model involved. When one fires, infuse asks
// the user before the action runs, in any permission mode, and records it in the journal.

import (
	"encoding/json"
	"path/filepath"
	"regexp"
)

var (
	reTestFile = regexp.MustCompile(`(?i)(^|/)(tests?|__tests__|specs?|e2e)/|[._-](test|spec)\.[a-z0-9]+$|_test\.go$|(^|/)test_[^/]+\.py$|(^|/)conftest\.py$`)
	// a request to make failing tests pass (not merely one that mentions tests)
	reMakePass = regexp.MustCompile(`(?i)(fa[çc]a|fazer|deix\w*|make|get)\s+(os\s+|the\s+)?(test\w*|ci)\s+(passar|pass\w*|verde|green)` +
		`|(test\w*|ci|specs?)\s+(est[ãa]o\s+|is\s+|are\s+)?(falhando|falhou|falham|quebrad\w*|failing|fails|failed|broken|red|vermelh\w*)` +
		`|(corrij\w*|corrigir|consert\w*|fix\w*)\s+(os?\s+|the\s+)?(test\w*|ci|build)` +
		`|(test\w*|ci)\s+(para\s+|to\s+)?pass` +
		`|\b(failing|broken|red)\s+(unit\s+)?tests?|testes?\s+(que\s+)?(falham|quebrad\w*)` +
		`|reprovou`)
)

// turnStart is the message that started the current turn.
func (s *Session) turnStart() (Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Turn < 1 || s.Turn-1 >= len(s.TurnStarts) || s.TurnStarts[s.Turn-1] >= len(s.Messages) {
		return Message{}, false
	}
	return s.Messages[s.TurnStarts[s.Turn-1]], true
}

// turnRequest is what the user asked for in the current turn: the text they typed, or, when infuse
// started the turn for a mission, the mission's goal (from the message's data, not its text).
func (s *Session) turnRequest() string {
	m, ok := s.turnStart()
	if !ok {
		return ""
	}
	if n, ok := missionNoteOf(m); ok {
		return n.Goal
	}
	return msgText(m)
}

// sentinel returns why an action needs the user's eyes, in the model's words, or "".
func (s *Session) sentinel(tool Tool, input json.RawMessage) string {
	return s.sentinelMsg(tool, input).String()
}

// sentinelMsg returns why an action needs the user's eyes (empty: it does not).
func (s *Session) sentinelMsg(tool Tool, input json.RawMessage) Msg {
	if tool.Category() != "write" {
		return Msg{}
	}
	var in struct {
		Path     string `json:"path"`
		FilePath string `json:"file_path"`
	}
	_ = json.Unmarshal(input, &in)
	p := in.Path
	if p == "" {
		p = in.FilePath
	}
	if p == "" || !reTestFile.MatchString(filepath.ToSlash(p)) {
		return Msg{}
	}
	m, _ := s.turnStart()
	if n, ok := missionNoteOf(m); ok && n.Verdict == "reprovado" { // the mission came back because Done? failed: the request is to make it pass
		return M("sentinel.test_edit")
	}
	if reMakePass.MatchString(s.turnRequest()) {
		return M("sentinel.test_edit")
	}
	return Msg{}
}
