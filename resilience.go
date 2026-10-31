package main

// A turn ends when the work is done — not because of a dropped connection, an overloaded provider, a
// reply cut at the output limit, a context that grew too large, a tool that panicked, a step counter, or
// infuse being restarted. Each of those is recovered here; the session stops only for what the user must
// act on (no provider, bad key, spending limit, cancel) or when the agent makes no progress at all.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

var errContextTooLong = errors.New("context too long")

// retryWaits are the pauses between attempts when the provider fails (about 6 minutes in all).
var retryWaits = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 15 * time.Second, 30 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second, 60 * time.Second}

// errorKind sorts a provider error: "transient" (retry), "context" (compact and retry) or "fatal".
func errorKind(err error) string {
	if err == nil {
		return ""
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return "transient"
	}
	m := strings.ToLower(err.Error())
	for _, k := range []string{"context length", "context_length", "maximum context", "too many tokens", "prompt is too long", "reduce the length", "context window"} {
		if strings.Contains(m, k) {
			return "context"
		}
	}
	for _, k := range []string{" 408", " 425", " 429", " 500", " 502", " 503", " 504", " 520", " 522", " 524", " 529", "overloaded", "rate limit", "timeout", "timed out", "connection reset", "connection refused", "broken pipe", "eof", "stream", "temporarily", "server error", "bad gateway", "no such host", "tls handshake"} {
		if strings.Contains(m, k) {
			return "transient"
		}
	}
	return "fatal"
}

// streamResilient calls the provider until it answers: transient failures are retried with growing
// waits (2s … 60s, about 6 minutes in all), an empty answer counts as a failure, and a context that
// is too long comes back as errContextTooLong for the caller to compact.
func (s *Session) streamResilient(ctx context.Context, p Provider, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	waits := retryWaits
	var lastErr error
	for attempt := 0; ; attempt++ {
		res, err := p.Stream(ctx, req, emit)
		if err == nil && len(res.Content) == 0 {
			err = errors.New("empty answer from the provider (stream)")
		}
		if err == nil {
			return res, nil
		}
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		switch errorKind(err) {
		case "context":
			return res, errContextTooLong
		case "fatal":
			return res, err
		}
		lastErr = err
		if attempt >= len(waits) {
			return res, fmt.Errorf("the provider kept failing for %d attempts: %w", attempt+1, lastErr)
		}
		wait := waits[attempt]
		s.emit("stream_reset", nil) // the partial answer of the failed attempt is discarded on screen
		s.emit("gate", map[string]any{"message": fmt.Sprintf("[infuse] The provider failed (%s); trying again in %ds (attempt %d).", firstLine(err.Error()), int(wait.Seconds()), attempt+2)})
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		case <-time.After(wait):
		}
	}
}

// outputLimit is how many tokens a reply may use: the model's official maximum, up to 64K (a reply
// cut short is recovered anyway, but should be rare).
func outputLimit(model string, configured int) int {
	if configured > 0 && configured != 16000 { // 16000 was a fixed default, never a choice
		return configured
	}
	l, ok := modelLimits[strings.ToLower(model)]
	if !ok {
		return 16000
	}
	return min(l.output, 64000)
}

// truncatedReply handles an answer cut at the output limit: calls whose input may be incomplete are
// dropped (never executed half-written) and the agent is told to continue from where it stopped.
func truncatedReply(res *ModelResult) (note string, cut bool) {
	if res.StopReason != "max_tokens" && res.StopReason != "length" {
		return "", false
	}
	var kept []Block
	dropped := 0
	for _, b := range res.Content {
		if b.Type == "tool_use" {
			dropped++
			continue
		}
		kept = append(kept, b)
	}
	if len(kept) == 0 {
		kept = []Block{{Type: "text", Text: "…"}}
	}
	res.Content = kept
	note = "[infuse] Your reply was cut at the output limit. Continue exactly where it stopped."
	if dropped > 0 {
		note += fmt.Sprintf(" %d tool call(s) in it were not run, because their input may be incomplete: make them again, in full (for a large file, write it in parts with Edit).", dropped)
	}
	return note, true
}

// compactFallback shrinks the conversation without a model when summarizing failed: the user's own
// words stay verbatim, the older tool outputs are cut to their first lines, the recent exchanges stay.
func (s *Session) compactFallback() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Messages) < 12 {
		return
	}
	keepFrom := len(s.Messages) - 10
	for keepFrom > 0 && s.Messages[keepFrom].Role != "user" {
		keepFrom--
	}
	older := s.Messages[:keepFrom]
	var b strings.Builder
	b.WriteString("[Contexto compactado — o resumo automático falhou; o infuse reduziu a conversa sem resumir]\n\n")
	if v := verbatimRequests(older); v != "" {
		b.WriteString("## Pedidos do usuário, literais\n" + v + "\n\n")
	}
	b.WriteString("## Ferramentas usadas antes deste ponto (primeiras linhas do resultado)\n")
	names := map[string]string{}
	for _, m := range older {
		for _, bl := range m.Content {
			if bl.Type == "tool_use" {
				names[bl.ID] = bl.Name + " " + truncate(string(bl.Input), 120)
			}
		}
	}
	for _, m := range older {
		for _, bl := range m.Content {
			if bl.Type == "tool_result" {
				fmt.Fprintf(&b, "- %s → %s\n", names[bl.ToolUseID], truncate(firstLine(bl.Content), 160))
			}
		}
	}
	keep := append([]Message{}, s.Messages[keepFrom:]...)
	s.Messages = append([]Message{{Role: "user", Content: []Block{{Type: "text", Text: truncate(b.String(), 60000)}}, TS: time.Now().UnixMilli()}, {Role: "assistant", Content: []Block{{Type: "text", Text: "Entendido, continuo a partir desse estado."}}}}, keep...)
	s.TurnStarts = []int{0}
	s.Compactions++
}

// compactResilient summarizes the conversation, falling back to compactFallback when the summary fails,
// and cuts the largest tool outputs when there is little to summarize but they alone fill the context.
func (s *Session) compactResilient(ctx context.Context, p Provider) {
	s.mu.Lock()
	before := s.Compactions
	s.mu.Unlock()
	if err := s.compact(ctx, p); err != nil && ctx.Err() == nil {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "compactação por resumo falhou: " + firstLine(err.Error())})
		s.compactFallback()
		s.emit("compacted", map[string]any{"summary": "(redução sem resumo: o provedor falhou ao resumir)"})
	}
	s.mu.Lock()
	compacted := s.Compactions > before
	s.mu.Unlock()
	if !compacted {
		s.shrinkLargestResults()
	}
}

// shrinkLargestResults keeps the head and tail of every tool output over 16K characters.
func (s *Session) shrinkLargestResults() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.Messages {
		for j, b := range s.Messages[i].Content {
			if b.Type == "tool_result" && len(b.Content) > 16000 {
				s.Messages[i].Content[j].Content = b.Content[:6000] + fmt.Sprintf("\n[... %d characters removed to fit the context ...]\n", len(b.Content)-12000) + b.Content[len(b.Content)-6000:]
			}
		}
	}
	s.Compactions++
}

// safeRunTool runs a tool; a panic inside it becomes that tool's error, not the end of the turn.
func (s *Session) safeRunTool(ctx context.Context, rules *RuleSet, b Block) (out string, isErr bool) {
	defer func() {
		if r := recover(); r != nil {
			out, isErr = fmt.Sprintf("[infuse] The tool %s failed inside infuse (%v). This is infuse's fault, not your input; try another way to do the same step.", b.Name, r), true
			Journal(s.Root, JournalEntry{Kind: "gate", Detail: fmt.Sprintf("panic em %s: %v", b.Name, r)})
		}
	}()
	return s.runTool(ctx, rules, b)
}

// progress is what the watchdog counts: a change landed, a plan task finished, a command passed.
type progressWatch struct {
	lastIter  int
	lastDone  int
	lastWrite int
	warned    bool
}

// stalled reports whether the turn made no progress for long; the first time it returns a note for the
// agent, the second time the reason to stop.
func (s *Session) stalled(w *progressWatch, iter int) (note string, stop string) {
	s.mu.Lock()
	done := 0
	for _, t := range s.Tasks {
		if t.Status == "done" || t.Status == "blocked" {
			done++
		}
	}
	writes := s.gate.writes
	s.mu.Unlock()
	if done != w.lastDone || writes != w.lastWrite {
		w.lastIter, w.lastDone, w.lastWrite, w.warned = iter, done, writes, false
		return "", ""
	}
	idle := iter - w.lastIter
	switch {
	case idle >= 120:
		return "", fmt.Sprintf("no progress in %d steps (no change landed and no plan task finished)", idle)
	case idle >= 60 && !w.warned:
		w.warned = true
		return fmt.Sprintf("[infuse] %d steps without progress: no change landed and no plan task finished. Stop exploring: state in the plan (TodoWrite) the next concrete change and make it, or mark the task blocked with the reason.", idle), ""
	}
	return "", ""
}

// ---------------------------------------------------------------- restarts

// ResumeInterrupted continues a turn that infuse stopped by closing or restarting.
func (s *Session) ResumeInterrupted() {
	s.mu.Lock()
	if s.State != "idle" && s.State != "error" && s.State != "" {
		s.mu.Unlock()
		return
	}
	s.settings = EffectiveSettings(s.Root)
	s.Messages = repairTranscript(s.Messages)
	note := Message{Role: "user", Content: []Block{{Type: "text", Text: "[infuse] infuse was restarted while you were working, and the turn was interrupted. Continue the task exactly from where it stopped: check the plan and the files you were changing, then go on."}}, TS: time.Now().UnixMilli()}
	if n := len(s.Messages); n > 0 && s.Messages[n-1].Role == "user" { // keep the roles alternating
		s.Messages[n-1].Content = append(s.Messages[n-1].Content, note.Content...)
	} else {
		s.Messages = append(s.Messages, note)
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.mu.Unlock()
	s.emit("turn", map[string]any{"turn": s.Turn, "resumed": true})
	s.save()
	go s.run(ctx)
}

// resumeInterruptedSessions continues, at startup, every session that was working when infuse stopped.
func resumeInterruptedSessions() {
	for _, p := range loadProjects() {
		for _, d := range listSessions(p.Root) {
			if !d.Working || d.Archived {
				continue
			}
			if s, err := loadSession(p.Root, d.ID); err == nil {
				Journal(p.Root, JournalEntry{Kind: "gate", Detail: "sessão retomada após reinício: " + d.ID})
				s.ResumeInterrupted()
			}
		}
	}
}

// markWorking records, on disk, whether a turn is in progress (read back at startup).
func (s *Session) markWorking(on bool) {
	s.mu.Lock()
	s.Working = on
	s.mu.Unlock()
	s.save()
}

var _ = json.Valid
