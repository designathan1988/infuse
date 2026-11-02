package agent

import (
	"context"
	"encoding/json"
	"sync"
	"time"
)

// Independent calls of one reply run at the same time. Order is what the model wrote: a read written
// after a write sees the write, so only calls next to each other form a group, and each group starts
// when everything before it has finished.

// parallelCategories are the tools that touch nothing and can share a moment. Network calls qualify
// once they need no question: reads outside the project ask, one question at a time.
var parallelCategories = map[string]bool{"read": true, "net": true}

// parallelSafe reports whether the call can run beside others of its group.
func (s *Session) parallelSafe(b Block) bool {
	t, ok := s.lookupTool(b.Name)
	if !ok {
		return false
	}
	if b.Name == "Agent" { // read-only subagents work side by side: an agent team
		var in struct{ Kind string }
		_ = json.Unmarshal(b.Input, &in)
		k, known := s.subagentKind(in.Kind)
		return known && k.ReadOnly
	}
	if !parallelCategories[t.Category()] || s.permissionFor(t, b.Input) != "allow" {
		return false
	}
	return s.mode() == "bypass" || !s.readsOutside(b.Input)
}

// parallelGroup counts the calls at the head of the list that can run together.
func (s *Session) parallelGroup(calls []Block, allowed func(string) bool) int {
	n := 0
	for _, b := range calls {
		if allowed != nil && !allowed(b.Name) || !s.parallelSafe(b) {
			break
		}
		n++
	}
	return n
}

// runReadsInParallel runs the first group of a reply's calls; see runParallelFrom.
func (s *Session) runReadsInParallel(ctx context.Context, rules *RuleSet, content []Block) map[string]Block {
	return s.runParallelFrom(ctx, rules, content, 0)
}

// runParallelFrom runs the group that starts at content[from], when it has two calls or more. It
// returns the results by tool_use id; calls it does not run are left to the sequential loop.
func (s *Session) runParallelFrom(ctx context.Context, rules *RuleSet, content []Block, from int) map[string]Block {
	out := map[string]Block{}
	calls := toolUses(content[from:])
	group := calls[:s.parallelGroup(calls, nil)]
	if len(group) < 2 {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, b := range group {
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
