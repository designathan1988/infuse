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
	if s.providerID() == "" {
		s.failMsg(M("session.no_provider"))
		return
	}
	provider, err := providerFor(s.settings, s.providerID())
	if err != nil {
		s.fail(err)
		return
	}
	rules := LoadRules(s.Root)
	mcp.For(s.Root).Ensure(ctx)          // approved connectors start before the first request
	s.ensureCompiledRules(ctx, provider) // the instructions' meaning, as quote-checked rules
	s.ensureKnowledge(ctx, provider)     // the project's documents, read to the last line
	s.compileBans(ctx, provider)         // what the user forbade, in the user's words, checked
	know := knowledgePrompt(s.Root)
	var breaker loopBreaker // stops a turn that keeps repeating the same failure
	stopHooks := 0
	var tripped error
	breakerTrips := 0 // the breaker warns, then demands research, then ends the turn
	var watch progressWatch
	maxOut := 0                          // raised when a reply is cut at the output limit
	contextRetries := 0                  // compactions in a row because the context did not fit
	for iter := 0; iter < 5000; iter++ { // a safety ceiling only: the watchdog decides when work stalled
		if note, stop := s.stalled(&watch, iter); !stop.Empty() {
			s.failMsg(stop)
			return
		} else if !note.Empty() {
			s.addNote("infuse_note", note)
		}
		if ctx.Err() != nil {
			s.finish("cancelado")
			return
		}
		if s.needsCompaction() {
			s.compactResilient(ctx, provider)
		}
		s.setState("thinking")
		budget := thinkingBudget(s.effort(), s.settings.ThinkingBudget)
		maxTok := max(outputLimit(s.Model, s.settings.MaxTokens), maxOut)
		if budget > 0 && maxTok <= budget+1024 {
			maxTok = budget + 8000
		}
		req := ModelRequest{Model: s.Model, System: buildSystemPrompt(s, rules) + "\n" + diagnosisRule + "\n\n" + s.workStatePrompt() + "\n\n" + know, Messages: repairTranscript(s.copyMessages()), Tools: s.toolSpecs(), MaxTokens: maxTok, Thinking: budget}
		started := time.Now()
		var thinkFrom, thinkTo time.Time // how long the model spent in its reasoning, from the stream
		res, err := s.streamResilient(ctx, provider, req, func(ev StreamEvent) {
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
				return
			}
			if errors.Is(err, errContextTooLong) { // the provider measured more than estimated: compact and go on
				if contextRetries++; contextRetries > 3 {
					s.failMsg(M("session.context_overflow", "error", err.Error()))
					return
				}
				s.compactResilient(ctx, provider)
				continue
			}
			contextRetries = 0
			s.fail(err)
			return
		}
		cutNote, cut := truncatedReply(&res) // a reply cut at the output limit is continued, never run half-written
		if cut {
			maxOut = min(max(maxTok*2, 32000), 128000)
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
		if lim := s.settings.DailyLimitUSD; lim > 0 { // the day's total across every session, from the ledger
			if spent := todaySpend(); spent > lim {
				s.failMsg(M("session.daily_limit", "spent", fmt.Sprintf("%.2f", spent), "limit", fmt.Sprintf("%.2f", lim)))
				return
			}
		}
		if cut {
			s.addNote("infuse_note", cutNote)
			continue
		}
		if res.StopReason != "tool_use" {
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
				continue
			}
			if stopHooks < 3 { // a Stop hook can send the agent back to work, with its reason
				if o := s.runHooks(ctx, "Stop", "", map[string]any{"stop_hook_active": stopHooks > 0}); o.Block {
					stopHooks++
					s.Mu.Lock()
					s.Messages = append(s.Messages, Message{Role: "user", Kind: "hook", Content: []Block{s.textBlockLocked(M("hook.stop", "message", o.Message))}, TS: time.Now().UnixMilli()})
					s.Mu.Unlock()
					s.emit("hook", map[string]any{"event": "Stop", "message": o.Message})
					continue
				}
			}
			s.finish("")
			return
		}
		var results []Block
		pre := s.runReadsInParallel(ctx, rules, res.Content) // several reads in one reply run at once
		for _, b := range res.Content {
			if b.Type != "tool_use" {
				continue
			}
			if r, ok := pre[b.ID]; ok {
				results = append(results, r)
				s.emit("tool_result", map[string]any{"id": b.ID, "name": b.Name, "output": r.Content, "display": r.Display, "isError": r.IsError, "ms": r.Ms})
				if err := breaker.record(b.Name, b.Input, r.IsError); err != nil {
					tripped = err
				}
				continue
			}
			toolStart := time.Now()
			out, isErr := s.safeRunTool(ctx, rules, b)
			out = redactSecrets(out) // credentials never reach the model, the transcript or the UI
			ms := time.Since(toolStart).Milliseconds()
			display := displayFor(s.uiLang(), out, s.takeCallMsgs(b.ID))
			results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr, Ms: ms, Display: display})
			s.Mu.Lock()
			s.inflight = append([]Block{}, results...)
			s.Mu.Unlock()
			s.emit("tool_result", map[string]any{"id": b.ID, "name": b.Name, "output": out, "display": display, "isError": isErr, "ms": ms})
			if err := breaker.record(b.Name, b.Input, isErr); err != nil {
				tripped = err
			}
			if ctx.Err() != nil || tripped != nil {
				break
			}
		}
		if ctx.Err() != nil || tripped != nil {
			why := M("session.cancelled")
			if tripped != nil {
				if why, _ = msgOf(tripped); why.Empty() {
					why = M("text.verbatim", "text", tripped.Error())
				}
			}
			s.Mu.Lock()
			// keep the conversation consistent: every tool_use gets a result
			for _, b := range res.Content {
				if b.Type == "tool_use" && !hasResult(results, b.ID) {
					r := s.textBlockLocked(why)
					results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: r.Text, Display: r.Display, IsError: true})
				}
			}
			s.Messages = append(s.Messages, Message{Role: "user", Content: results, TS: time.Now().UnixMilli()})
			s.inflight = nil
			s.Mu.Unlock()
			if tripped != nil && breakerTrips == 1 && ctx.Err() == nil { // the second time: research outside the project first
				breakerTrips++
				breaker = loopBreaker{}
				note := M("breaker.second", "why", why)
				tripped = nil
				s.Mu.Lock()
				s.gate.fails, s.gate.needWeb = 2, true // the research gate now accepts only outside research
				s.gate.diagnosed, s.gate.redoWhy = false, M("redo.breaker")
				last := &s.Messages[len(s.Messages)-1]
				last.Content = append(last.Content, s.textBlockLocked(note))
				s.Mu.Unlock()
				s.emitMsg("gate", note)
				continue
			}
			if tripped != nil && breakerTrips == 0 && ctx.Err() == nil {
				// the first time, the agent is sent back with an order to change approach; only insisting ends the turn
				breakerTrips++
				breaker = loopBreaker{}
				note := M("breaker.first", "why", why)
				tripped = nil
				s.Mu.Lock() // in the same message as the results: the roles keep alternating
				last := &s.Messages[len(s.Messages)-1]
				last.Content = append(last.Content, s.textBlockLocked(note))
				s.Mu.Unlock()
				s.emitMsg("gate", note)
				continue
			}
			if tripped != nil {
				Lesson(s.Root, "loop-errático", tripped.Error())
				s.failMsg(why)
				return
			}
			s.finish("cancelado")
			return
		}
		s.Mu.Lock()
		results = append(results, s.pendingImages...) // images opened with Read, after the tool results
		s.pendingImages = nil
		s.Messages = append(s.Messages, Message{Role: "user", Content: results, TS: time.Now().UnixMilli()})
		s.inflight = nil
		s.Mu.Unlock()
		s.save()
	}
	s.failMsg(M("session.iteration_limit"))
}

// addNote adds a note of infuse's to the conversation (English for the model, the user's language on
// screen) and tells the user.
