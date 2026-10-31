package main

// Sentinels: deterministic checks on the agent's actions, no model involved. When one fires, infuse asks
// the user before the action runs, in any permission mode, and records it in the journal.

import (
	"encoding/json"
	"path/filepath"
	"regexp"
)

var (
	reTestFile  = regexp.MustCompile(`(?i)(^|/)(tests?|__tests__|specs?|e2e)/|[._-](test|spec)\.[a-z0-9]+$|_test\.go$|(^|/)test_[^/]+\.py$|(^|/)conftest\.py$`)
	reMakePass  = regexp.MustCompile(`(?i)\b(pass(ar|e|em|ando)?|verde|green|falh\w*|quebr\w*|fail\w*|fix\w*|corrig\w*|consert\w*)\b`)
	reMentTests = regexp.MustCompile(`(?i)\btest(e|es|s)?\b|\bspec\b|\bci\b`)
)

const sentinelTestEdit = "alteração em arquivo de teste num pedido para os testes passarem"

// turnRequest is the text the user typed to start the current turn.
func (s *Session) turnRequest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Turn < 1 || s.Turn-1 >= len(s.TurnStarts) || s.TurnStarts[s.Turn-1] >= len(s.Messages) {
		return ""
	}
	return msgText(s.Messages[s.TurnStarts[s.Turn-1]])
}

// sentinel returns why an action needs the user's eyes, or "".
func (s *Session) sentinel(tool Tool, input json.RawMessage) string {
	if tool.Category() != "write" {
		return ""
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
		return ""
	}
	if req := s.turnRequest(); reMakePass.MatchString(req) && reMentTests.MatchString(req) {
		return sentinelTestEdit
	}
	return ""
}
