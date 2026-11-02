package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Permission modes, asks and user answers.

func (s *Session) permissionFor(tool Tool, input json.RawMessage) string {
	name := tool.Spec().Name
	cat := tool.Category()
	rule := s.patternDecision(name, input)
	if rule == "deny" { // a deny rule holds in every mode
		return "deny"
	}
	mode := s.mode()
	if mode == "bypass" {
		return "allow"
	}
	if mode == "plan" && (cat == "write" || cat == "exec") {
		return "deny"
	}
	if rule != "" {
		return rule
	}
	if mode == "acceptEdits" && (cat == "write" || cat == "memory") {
		return "allow"
	}
	if cat == "read" || cat == "ui" {
		return "allow"
	}
	if s.alwaysAllowed(name, input) {
		return "allow"
	}
	if v, ok := s.settings.Permissions[name]; ok {
		return v
	}
	return "ask"
}

func (s *Session) askPermission(ctx context.Context, tool Tool, input json.RawMessage) bool {
	return s.askPermissionWhy(ctx, tool, input, "", Msg{})
}

// askPermissionWhy asks the user about a call a sentinel caught: reason is shown in the user's language;
// key identifies the sentinel, so "don't ask again" silences that sentinel for the session.
func (s *Session) askPermissionWhy(ctx context.Context, tool Tool, input json.RawMessage, key string, reason Msg) bool {
	if s.unattended() {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "refused, nobody to approve it: " + tool.Spec().Name + " " + reason.Render("en")})
		return false
	}
	s.askMu.Lock()
	defer s.askMu.Unlock()
	asked := time.Now()
	defer func() { // the user's decision time is not the tool's run time
		s.Mu.Lock()
		s.permWaitNs += time.Since(asked).Nanoseconds()
		s.Mu.Unlock()
	}()
	req := &permRequest{ID: fmt.Sprintf("p%d", time.Now().UnixNano()), Tool: tool.Spec().Name, Input: input, Category: tool.Category(), Reason: reason.Render(s.uiLang()), Prefix: bashPrefix(tool.Spec().Name, input), reply: make(chan string, 1)}
	s.Mu.Lock()
	s.pendingPerm = req
	s.Mu.Unlock()
	s.setState("waiting_permission")
	s.emit("permission", req)
	s.notificationHooks("permission_prompt", req.Tool)
	defer func() { s.Mu.Lock(); s.pendingPerm = nil; s.Mu.Unlock() }()
	select {
	case d := <-req.reply:
		if d == "always" && key != "" { // "don't ask again" silences this sentinel for the session, not the tool
			s.Mu.Lock()
			s.allowAlways["sentinel:"+key] = true
			s.Mu.Unlock()
		} else if d == "always" { // Bash: the prefix the question showed, never all of Bash
			s.Mu.Lock()
			if req.Prefix != "" {
				s.allowAlways[alwaysKey(req.Prefix)] = true
			} else if req.Tool != "Bash" {
				s.allowAlways[req.Tool] = true
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

// unattended: nobody is there to answer. A question fails at once instead of holding the turn forever.
func (s *Session) unattended() bool {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.Unattended
}

func (s *Session) askUser(ctx context.Context, question string, options []string) (string, bool) {
	if s.unattended() {
		return "", false
	}
	s.askMu.Lock()
	defer s.askMu.Unlock()
	req := &askRequest{ID: fmt.Sprintf("q%d", time.Now().UnixNano()), Question: question, Options: options, reply: make(chan string, 1)}
	s.Mu.Lock()
	s.pendingAsk = req
	s.Mu.Unlock()
	s.setState("waiting_user")
	s.emit("question", req)
	s.notificationHooks("question", question)
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
