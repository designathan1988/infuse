package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Permission modes, asks and user answers.

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
	s.Mu.Lock()
	always := s.allowAlways[name] || s.allowAlways[name+":"+bashPrefix(name, input)]
	s.Mu.Unlock()
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
	return s.askPermissionWhy(ctx, tool, input, "", Msg{})
}

// askPermissionWhy asks the user about a call a sentinel caught: reason is shown in the user's language;
// key identifies the sentinel, so "don't ask again" silences that sentinel for the session.
func (s *Session) askPermissionWhy(ctx context.Context, tool Tool, input json.RawMessage, key string, reason Msg) bool {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	asked := time.Now()
	defer func() { // the user's decision time is not the tool's run time
		s.Mu.Lock()
		s.permWaitNs += time.Since(asked).Nanoseconds()
		s.Mu.Unlock()
	}()
	req := &permRequest{ID: fmt.Sprintf("p%d", time.Now().UnixNano()), Tool: tool.Spec().Name, Input: input, Category: tool.Category(), Reason: reason.Render(s.uiLang()), reply: make(chan string, 1)}
	s.Mu.Lock()
	s.pendingPerm = req
	s.Mu.Unlock()
	s.setState("waiting_permission")
	s.emit("permission", req)
	defer func() { s.Mu.Lock(); s.pendingPerm = nil; s.Mu.Unlock() }()
	select {
	case d := <-req.reply:
		if d == "always" && key != "" { // "don't ask again" silences this sentinel for the session, not the tool
			s.Mu.Lock()
			s.allowAlways["sentinel:"+key] = true
			s.Mu.Unlock()
		} else if d == "always" {
			s.Mu.Lock()
			s.allowAlways[req.Tool] = true
			if p := bashPrefix(req.Tool, input); p != "" {
				s.allowAlways[req.Tool+":"+p] = true
			}
			s.Mu.Unlock()
		}
		return d == "allow" || d == "always"
	case <-ctx.Done():
		return false
	}
}

func (s *Session) ResolvePermission(id, decision string) bool {
	s.Mu.Lock()
	req := s.pendingPerm
	s.Mu.Unlock()
	if req == nil || req.ID != id {
		return false
	}
	req.reply <- decision
	return true
}

func (s *Session) askUser(ctx context.Context, question string, options []string) (string, bool) {
	s.askMu.Lock()
	defer s.askMu.Unlock()
	req := &askRequest{ID: fmt.Sprintf("q%d", time.Now().UnixNano()), Question: question, Options: options, reply: make(chan string, 1)}
	s.Mu.Lock()
	s.pendingAsk = req
	s.Mu.Unlock()
	s.setState("waiting_user")
	s.emit("question", req)
	defer func() { s.Mu.Lock(); s.pendingAsk = nil; s.Mu.Unlock() }()
	select {
	case a := <-req.reply:
		return a, true
	case <-ctx.Done():
		return "", false
	}
}

func (s *Session) Answer(id, answer string) bool {
	s.Mu.Lock()
	req := s.pendingAsk
	s.Mu.Unlock()
	if req == nil || req.ID != id {
		return false
	}
	req.reply <- answer
	return true
}

// ---------------------------------------------------------------- the loop

// userBlocks builds the user message: the text, then each attached file (images as image blocks, other
// files as their content, with the path), then pasted images.
