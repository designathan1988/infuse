package agent

// The hook events outside the tool loop: SessionStart, PreCompact and Notification.

import (
	"context"
	"time"
)

// hookContextEvents are the events whose hook's standard output becomes context for the agent.
var hookContextEvents = map[string]bool{"UserPromptSubmit": true, "SessionStart": true}

// sessionStartHooks runs once for each session the process opens (matcher: startup or resume). What the
// hooks print joins the system prompt.
func (s *Session) sessionStartHooks(ctx context.Context) {
	s.startHooks.Do(func() {
		s.Mu.Lock()
		source := "startup"
		if len(s.Messages) > 1 {
			source = "resume"
		}
		s.Mu.Unlock()
		o := s.runHooks(ctx, "SessionStart", source, map[string]any{"source": source})
		s.Mu.Lock()
		s.startContext = o.Message
		s.Mu.Unlock()
	})
}

func (s *Session) sessionStartContext() string {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	return s.startContext
}

// preCompactHooks tells the hooks a compaction is about to replace the conversation (matcher: auto).
// A hook cannot stop it: the alternative is a request that no longer fits.
func (s *Session) preCompactHooks(ctx context.Context, trigger string) {
	s.Mu.Lock()
	n := len(s.Messages)
	s.Mu.Unlock()
	s.runHooks(ctx, "PreCompact", trigger, map[string]any{"trigger": trigger, "messages": n})
}

// notificationHooks tells the hooks the agent needs the user (matcher: the kind of notification). It
// runs on the side: a notification never holds the question it announces.
func (s *Session) notificationHooks(kind, message string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		s.runHooks(ctx, "Notification", kind, map[string]any{"notification_type": kind, "message": message})
	}()
}
