package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// runReadsInParallel runs the read-only calls of one reply concurrently when the reply has more than one
// and none of them needs to ask the user (reads outside the project ask, one question at a time).
// It returns the results by tool_use id; calls it does not run are left to the sequential loop.
func (s *Session) runReadsInParallel(ctx context.Context, rules *RuleSet, content []Block) map[string]Block {
	var reads []Block
	for _, b := range content {
		if b.Type != "tool_use" {
			continue
		}
		t, ok := s.lookupTool(b.Name)
		if ok && b.Name == "Agent" { // read-only subagents (explore, review) work side by side: an agent team
			var in struct{ Kind string }
			_ = json.Unmarshal(b.Input, &in)
			if in.Kind == "explore" || in.Kind == "review" {
				reads = append(reads, b)
			}
			continue
		}
		if !ok || t.Category() != "read" || s.permissionFor(t, b.Input) != "allow" || (s.mode() != "bypass" && s.readsOutside(b.Input)) {
			continue
		}
		reads = append(reads, b)
	}
	out := map[string]Block{}
	if len(reads) < 2 {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, b := range reads {
		wg.Add(1)
		go func(b Block) {
			defer wg.Done()
			start := time.Now()
			res, isErr := s.runTool(ctx, rules, b)
			res = redactSecrets(res)
			// the user's version: infuse's messages recorded for this call, in the user's language
			r := Block{Type: "tool_result", ToolUseID: b.ID, Content: res, IsError: isErr, Ms: time.Since(start).Milliseconds(), Display: displayFor(s.uiLang(), res, s.takeCallMsgs(b.ID))}
			mu.Lock()
			out[b.ID] = r
			mu.Unlock()
		}(b)
	}
	wg.Wait()
	return out
}
