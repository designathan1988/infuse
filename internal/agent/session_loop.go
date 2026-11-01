package agent

import (
	"context"
	"errors"
	"fmt"
	"infuse/internal/mcp"
	"strings"
	"time"
)

// The agent loop: request, stream, tools, obligations.

type runState struct {
	provider       Provider
	rules          *RuleSet
	know           string
	breaker        loopBreaker
	stopHooks      int
	tripped        error
	breakerTrips   int
	watch          progressWatch
	maxOut         int
	contextRetries int
	cut            bool
	cutNote        Msg
}

func (s *Session) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			s.Mu.Lock()
			s.lastError = fmt.Sprint(r)
			s.Mu.Unlock()
			s.setState("error")
		}
		s.save()
	}()
	st, ok := s.setupRun(ctx)
	if !ok {
		return
	}
	for iter := 0; iter < 5000; iter++ { // a safety ceiling only: the watchdog decides when work stalled
		if s.runPrelude(ctx, st, iter) {
			return
		}
		res, outcome := s.request(ctx, st)
		if outcome == requestStop {
			return
		}
		if outcome == requestRetry {
			continue
		}
		if st.cut {
			s.addNote("infuse_note", st.cutNote)
			continue
		}
		if res.StopReason != "tool_use" {
			if s.endTurn(ctx, st) {
				return
			}
			continue
		}
		results := s.executeTools(ctx, st, res)
		if s.settleTools(ctx, st, res, results) {
			return
		}
	}
	s.failMsg(M("session.iteration_limit"))
}

func (s *Session) setupRun(ctx context.Context) (*runState, bool) {
	if s.providerID() == "" {
		s.failMsg(M("session.no_provider"))
		return nil, false
	}
	provider, err := providerFor(s.settings, s.providerID())
	if err != nil {
		s.fail(err)
		return nil, false
	}
	rules := LoadRules(s.Root)
	mcp.For(s.Root).Ensure(ctx)          // approved connectors start before the first request
	s.ensureCompiledRules(ctx, provider) // the instructions' meaning, as quote-checked rules
	s.ensureKnowledge(ctx, provider)     // the project's documents, read to the last line
	s.compileBans(ctx, provider)         // what the user forbade, in the user's words, checked
	return &runState{provider: provider, rules: rules, know: knowledgePrompt(s.Root)}, true
}

func (s *Session) runPrelude(ctx context.Context, st *runState, iter int) bool {
	if note, stop := s.stalled(&st.watch, iter); !stop.Empty() {
		s.failMsg(stop)
		return true
	} else if !note.Empty() {
		s.addNote("infuse_note", note)
	}
	if ctx.Err() != nil {
		s.finish("cancelado")
		return true
	}
	if s.needsCompaction() {
		s.compactResilient(ctx, st.provider)
	}
	return false
}

type requestOutcome int

const (
	requestOK requestOutcome = iota
	requestRetry
	requestStop
)

func (s *Session) request(ctx context.Context, st *runState) (ModelResult, requestOutcome) {
	s.setState("thinking")
	budget := thinkingBudget(s.effort(), s.settings.ThinkingBudget)
	maxTok := max(outputLimit(s.Model, s.settings.MaxTokens), st.maxOut)
	if budget > 0 && maxTok <= budget+1024 {
		maxTok = budget + 8000
	}
	req := ModelRequest{Model: s.Model, System: buildSystemPrompt(s, st.rules) + "\n" + diagnosisRule + "\n\n" + s.workStatePrompt() + "\n\n" + st.know, Messages: repairTranscript(s.copyMessages()), Tools: s.toolSpecs(), MaxTokens: maxTok, Thinking: budget}
	started := time.Now()
	var thinkFrom, thinkTo time.Time // how long the model spent in its reasoning, from the stream
	res, err := s.streamResilient(ctx, st.provider, req, func(ev StreamEvent) {
		if ev.Kind == "thinking" && thinkFrom.IsZero() {
			thinkFrom = time.Now()
		} else if ev.Kind != "thinking" && !thinkFrom.IsZero() && thinkTo.IsZero() {
			thinkTo = time.Now()
		}
		s.emit("stream", ev)
	})
	if err != nil {
		if ctx.Err() != nil {
			s.finish("cancelado")
			return res, requestStop
		}
		if errors.Is(err, errContextTooLong) { // the provider measured more than estimated: compact and go on
			if st.contextRetries++; st.contextRetries > 3 {
				s.failMsg(M("session.context_overflow", "error", err.Error()))
				return res, requestStop
			}
			s.compactResilient(ctx, st.provider)
			return res, requestRetry
		}
		st.contextRetries = 0
		s.fail(err)
		return res, requestStop
	}
	st.contextRetries = 0
	st.cutNote, st.cut = truncatedReply(&res) // a reply cut at the output limit is continued, never run half-written
	if st.cut {
		st.maxOut = min(max(maxTok*2, 32000), 128000)
	}
	if !thinkFrom.IsZero() {
		if thinkTo.IsZero() {
			thinkTo = time.Now()
		}
		for i := range res.Content {
			if res.Content[i].Type == "thinking" {
				res.Content[i].Ms = thinkTo.Sub(thinkFrom).Milliseconds()
			}
		}
	}
	s.recordReply(res, started)
	if lim := s.settings.DailyLimitUSD; lim > 0 { // the day's total across every session, from the ledger
		if spent := todaySpend(); spent > lim {
			s.failMsg(M("session.daily_limit", "spent", fmt.Sprintf("%.2f", spent), "limit", fmt.Sprintf("%.2f", lim)))
			return res, requestStop
		}
	}
	return res, requestOK
}

func (s *Session) recordReply(res ModelResult, started time.Time) {
	reply := Message{Role: "assistant", Content: res.Content, TS: time.Now().UnixMilli(), Ms: time.Since(started).Milliseconds(), Model: s.Model, Tokens: res.Usage.Output}
	s.Mu.Lock()
	s.Messages = append(s.Messages, reply)
	s.Usage.Input += res.Usage.Input
	s.Usage.Output += res.Usage.Output
	s.Usage.CacheRead += res.Usage.CacheRead
	s.Usage.CacheWrite += res.Usage.CacheWrite
	cost := s.settings.CostOf(s.Model, res.Usage)
	s.Cost += cost
	s.LastInput = res.Usage.Input + res.Usage.CacheRead + res.Usage.CacheWrite
	if s.mode() == "plan" && res.StopReason != "tool_use" {
		for _, b := range res.Content {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				s.Plan = b.Text
			}
		}
	}
	s.Mu.Unlock()
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.Model, Input: res.Usage.Input, Output: res.Usage.Output, Cache: res.Usage.CacheRead + res.Usage.CacheWrite, Cost: cost})
	s.emit("message", map[string]any{"message": reply, "usage": res.Usage, "totalUsage": s.Usage, "cost": s.Cost, "lastInput": res.Usage.Input + res.Usage.CacheRead + res.Usage.CacheWrite})
	s.save()
}

func (s *Session) endTurn(ctx context.Context, st *runState) bool {
	why := s.workObligation()
	if why.Empty() {
		why = s.turnObligation(ctx)
	}
	if !why.Empty() { // the protocol decides when the turn may end
		s.Mu.Lock()
		s.gate.obligationAsks++
		if why.ID == "gate.no_report" {
			s.gate.reportAsks++
		}
		s.Mu.Unlock()
		kind := "infuse_note"
		if why.ID == "turn.review" {
			kind = "review"
		}
		s.addNote(kind, why)
		return false
	}
	if st.stopHooks < 3 { // a Stop hook can send the agent back to work, with its reason
		if o := s.runHooks(ctx, "Stop", "", map[string]any{"stop_hook_active": st.stopHooks > 0}); o.Block {
			st.stopHooks++
			s.Mu.Lock()
			s.Messages = append(s.Messages, Message{Role: "user", Kind: "hook", Content: []Block{s.textBlockLocked(M("hook.stop", "message", o.Message))}, TS: time.Now().UnixMilli()})
			s.Mu.Unlock()
			s.emit("hook", map[string]any{"event": "Stop", "message": o.Message})
			return false
		}
	}
	s.finish("")
	return true
}

func (s *Session) executeTools(ctx context.Context, st *runState, res ModelResult) []Block {
	var results []Block
	pre := s.runReadsInParallel(ctx, st.rules, res.Content) // several reads in one reply run at once
	for _, b := range res.Content {
		if b.Type != "tool_use" {
			continue
		}
		if r, ok := pre[b.ID]; ok {
			results = append(results, r)
			s.emit("tool_result", map[string]any{"id": b.ID, "name": b.Name, "output": r.Content, "display": r.Display, "isError": r.IsError, "ms": r.Ms})
			if err := st.breaker.record(b.Name, b.Input, r.IsError); err != nil {
				st.tripped = err
			}
			continue
		}
		toolStart := time.Now()
		s.Mu.Lock()
		s.permWaitNs = 0 // only this tool's approval wait is subtracted
		s.Mu.Unlock()
		out, isErr := s.safeRunTool(ctx, st.rules, b)
		out = redactSecrets(out) // credentials never reach the model, the transcript or the UI
		s.Mu.Lock()
		ms := time.Since(toolStart).Milliseconds() - s.permWaitNs/int64(time.Millisecond)
		s.permWaitNs = 0
		s.Mu.Unlock()
		if ms < 0 {
			ms = 0
		}
		display := displayFor(s.uiLang(), out, s.takeCallMsgs(b.ID))
		results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr, Ms: ms, Display: display})
		s.Mu.Lock()
		s.inflight = append([]Block{}, results...)
		s.Mu.Unlock()
		s.emit("tool_result", map[string]any{"id": b.ID, "name": b.Name, "output": out, "display": display, "isError": isErr, "ms": ms})
		if err := st.breaker.record(b.Name, b.Input, isErr); err != nil {
			st.tripped = err
		}
		if ctx.Err() != nil || st.tripped != nil {
			break
		}
	}
	return results
}

// settleTools closes the tool turn. It returns true when the run must end; false continues the loop.
func (s *Session) settleTools(ctx context.Context, st *runState, res ModelResult, results []Block) bool {
	if ctx.Err() == nil && st.tripped == nil {
		s.closeToolTurn(res, results)
		return false
	}
	why := M("session.cancelled")
	if st.tripped != nil {
		if why, _ = msgOf(st.tripped); why.Empty() {
			why = M("text.verbatim", "text", st.tripped.Error())
		}
	}
	s.closeToolTurn(res, results, why)
	return s.settleTrip(ctx, st, why)
}

// closeToolTurn appends the tool results as a user message; why fills the missing results when set.
func (s *Session) closeToolTurn(res ModelResult, results []Block, why ...Msg) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	if len(why) > 0 {
		// keep the conversation consistent: every tool_use gets a result
		for _, b := range res.Content {
			if b.Type == "tool_use" && !hasResult(results, b.ID) {
				r := s.textBlockLocked(why[0])
				results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: r.Text, Display: r.Display, IsError: true})
			}
		}
	} else {
		results = append(results, s.pendingImages...) // images opened with Read, after the tool results
		s.pendingImages = nil
	}
	s.Messages = append(s.Messages, Message{Role: "user", Content: results, TS: time.Now().UnixMilli()})
	s.inflight = nil
	s.save()
}

func (s *Session) settleTrip(ctx context.Context, st *runState, why Msg) bool {
	if st.tripped != nil && st.breakerTrips == 1 && ctx.Err() == nil { // the second time: research outside the project first
		st.breakerTrips++
		st.breaker = loopBreaker{}
		note := M("breaker.second", "why", why)
		st.tripped = nil
		s.Mu.Lock()
		s.gate.fails, s.gate.needWeb = 2, true // the research gate now accepts only outside research
		s.gate.diagnosed, s.gate.redoWhy = false, M("redo.breaker")
		last := &s.Messages[len(s.Messages)-1]
		last.Content = append(last.Content, s.textBlockLocked(note))
		s.Mu.Unlock()
		s.emitMsg("gate", note)
		return false
	}
	if st.tripped != nil && st.breakerTrips == 0 && ctx.Err() == nil {
		// the first time, the agent is sent back with an order to change approach; only insisting ends the turn
		st.breakerTrips++
		st.breaker = loopBreaker{}
		note := M("breaker.first", "why", why)
		st.tripped = nil
		s.Mu.Lock() // in the same message as the results: the roles keep alternating
		last := &s.Messages[len(s.Messages)-1]
		last.Content = append(last.Content, s.textBlockLocked(note))
		s.Mu.Unlock()
		s.emitMsg("gate", note)
		return false
	}
	if st.tripped != nil {
		Lesson(s.Root, "loop-errático", st.tripped.Error())
		s.failMsg(why)
		return true
	}
	s.finish("cancelado")
	return true
}

// addNote adds a note of infuse's to the conversation (English for the model, the user's language on
// screen) and tells the user.
