package main

// Sentinels: deterministic checks on the agent's actions, no model involved. When one fires, infuse asks
// the user before the action runs, in any permission mode, and records it in the journal.

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
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

const sentinelTestEdit = "a change to a test file in a request to make the tests pass"

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
	req := s.turnRequest()
	if strings.HasPrefix(req, "[Missão]") || strings.HasPrefix(req, "[Mission]") { // infuse's own mission instructions are not the user's request
		for _, goal := range []string{"Objetivo: ", "Goal: "} {
			if k := strings.Index(req, goal); k >= 0 {
				req = req[k+len(goal):]
				break
			}
		}
	}
	if reMakePass.MatchString(req) {
		return sentinelTestEdit
	}
	return ""
}
