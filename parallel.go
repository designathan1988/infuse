package main

import (
	"context"
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
		t, ok := toolRegistry[b.Name]
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
			r := Block{Type: "tool_result", ToolUseID: b.ID, Content: redactSecrets(res), IsError: isErr, Ms: time.Since(start).Milliseconds()}
			mu.Lock()
			out[b.ID] = r
			mu.Unlock()
		}(b)
	}
	wg.Wait()
	return out
}
