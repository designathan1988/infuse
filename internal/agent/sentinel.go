package agent

// Sentinels: deterministic checks on the agent's actions, no model involved. When one fires, the action
// is refused in every permission mode, with no card to approve it, and recorded in the journal.

import (
	"encoding/json"
	"infuse/internal/lang"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reTestDisabling = regexp.MustCompile(`(?i)(\.skip\s*\(|\.only\s*\(|\bxit\s*\(|\bxdescribe\s*\(|\bxtest\s*\()`)
	reTestDecl      = regexp.MustCompile(`(?m)^\s*(?:test|it|describe|func\s+Test|def\s+test_|#\[test\])`)
	reAssertion     = regexp.MustCompile(`(?m)\b(?:assert|expect|require)\s*\(|\bassert\.\w+\(|\bt\.(?:Error|Errorf|Fatal|Fatalf|Fail|FailNow)\(|^\s*assert\s+\S`)
)

var reTestFile = lang.TestPath

// turnStart is the message that started the current turn.
func (s *Session) turnStart() (Message, bool) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if s.Turn < 1 {
		return Message{}, false
	}
	if s.Turn-1 < len(s.TurnStarts) && s.TurnStarts[s.Turn-1] < len(s.Messages) {
		return s.Messages[s.TurnStarts[s.Turn-1]], true
	}
	// Compaction replaces the transcript and resets TurnStarts; the request is still the last user
	// message, and the Contract/obligation gates must keep working after it.
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == "user" {
			return s.Messages[i], true
		}
	}
	return Message{}, false
}

// turnRequest is what the user asked for in the current turn: the text they typed, or, when infuse
// started the turn for a mission, the mission's goal (from the message's data, not its text).
func (s *Session) turnRequest() string {
	s.Mu.Lock()
	stored := s.TurnRequest
	s.Mu.Unlock()
	if stored != "" { // kept apart from the transcript: compaction may have summarized the message
		return stored
	}
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
		Path      string `json:"path"`
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
		Content   string `json:"content"`
		Edits     []struct {
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(input, &in)
	p := in.Path
	if p == "" {
		p = in.FilePath
	}
	if p == "" || !reTestFile.MatchString(filepath.ToSlash(p)) {
		return Msg{}
	}
	// An existing test is the user's oracle: removing or changing it needs the user's yes. Adding new
	// tests (pure additions) is legitimate work and does not.
	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(s.Cwd, abs)
	}
	if s.agentFile(relTo(s.workRoot(), abs)) {
		return Msg{} // the agent created this test in this session; it may finish it
	}
	existing, err := os.ReadFile(abs)
	if err != nil {
		return Msg{} // a new test file is free
	}
	after, ok := appliedTestEdit(string(existing), in.OldString, in.NewString, in.Content, in.Edits)
	if !ok || testWeakeningEdit(string(existing), after) {
		return M("sentinel.test_edit")
	}
	return Msg{}
}

// appliedTestEdit reconstructs the file content a Write/Edit/MultiEdit would produce, so the guard
// can judge the result instead of one fragment of it.
func appliedTestEdit(existing, oldString, newString, content string, edits []struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}) (string, bool) {
	if content != "" {
		return content, true
	}
	if len(edits) > 0 {
		out := existing
		for _, e := range edits {
			if e.OldString == "" {
				out = e.NewString + out
				continue
			}
			if !strings.Contains(out, e.OldString) {
				return "", false
			}
			out = strings.Replace(out, e.OldString, e.NewString, 1)
		}
		return out, true
	}
	if oldString != "" || newString != "" {
		if !strings.Contains(existing, oldString) {
			return "", false
		}
		return strings.Replace(existing, oldString, newString, 1), true
	}
	return "", false
}

// testWeakeningEdit reports fraud, not correction: disabling tests or removing test cases and
// assertions. Changing an expected value keeps the test's power and is allowed.
func testWeakeningEdit(before, after string) bool {
	if before == after {
		return false
	}
	if len(reTestDisabling.FindAllString(after, -1)) > len(reTestDisabling.FindAllString(before, -1)) {
		return true
	}
	if len(reTestDecl.FindAllString(after, -1)) < len(reTestDecl.FindAllString(before, -1)) {
		return true
	}
	if len(reAssertion.FindAllString(after, -1)) < len(reAssertion.FindAllString(before, -1)) {
		return true
	}
	return false
}

func (s *Session) markAgentFile(rel string) {
	rel = filepath.ToSlash(rel)
	s.Mu.Lock()
	if s.AgentFiles == nil {
		s.AgentFiles = map[string]bool{}
	}
	s.AgentFiles[rel] = true
	s.Mu.Unlock()
	s.save()
}

func (s *Session) agentFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.AgentFiles[rel]
}

// additiveTestEdit reports whether every change only adds text to the existing test file. The added
// text is rejected when it contains a test-disabling marker, which is the common way to fake a pass.
func additiveTestEdit(oldString, newString, content string, edits []struct {
	OldString string `json:"old_string"`
	NewString string `json:"new_string"`
}, existing []byte) bool {
	added := []string{}
	if len(edits) > 0 {
		for _, e := range edits {
			if e.OldString == "" || !strings.Contains(e.NewString, e.OldString) {
				return false
			}
			added = append(added, strings.ReplaceAll(e.NewString, e.OldString, ""))
		}
	} else if oldString != "" || newString != "" {
		if oldString != "" && !strings.Contains(newString, oldString) {
			return false
		}
		added = append(added, strings.ReplaceAll(newString, oldString, ""))
	} else if content != "" {
		return additiveContent(string(existing), content)
	} else {
		return false
	}
	for _, a := range added {
		if reTestDisabling.MatchString(a) {
			return false
		}
	}
	return true
}

// additiveContent reports whether after only adds text to before, with no test-disabling marker in
// the added part. It is shared by the Write/Edit sentinel and the shell-write audit.
func additiveContent(before, after string) bool {
	if before == "" {
		return true
	}
	if !strings.Contains(after, before) {
		return false
	}
	return !reTestDisabling.MatchString(strings.ReplaceAll(after, before, ""))
}
