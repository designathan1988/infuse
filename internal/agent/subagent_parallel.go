package agent

import (
	"context"
	"encoding/json"
	"sync"
)

// A subagent's own tool calls: read-only calls of one reply run side by side, in the same groups the
// main loop uses; anything else runs alone, in its place.

// callRecorder records a finished call with the loop breaker; parallel calls share it.
type callRecorder func(name string, input json.RawMessage, isErr bool) error

func lockedRecorder(b *loopBreaker) callRecorder {
	var mu sync.Mutex
	return func(name string, input json.RawMessage, isErr bool) error {
		mu.Lock()
		defer mu.Unlock()
		return b.record(name, input, isErr)
	}
}

func (s *Session) subagentToolTurn(ctx context.Context, rules *RuleSet, allowed func(string) bool, content []Block, run int, id string, breaker *loopBreaker) ([]Block, error) {
	calls := toolUses(content)
	record := lockedRecorder(breaker)
	results := make([]Block, len(calls))
	errs := make([]error, len(calls))
	runOne := func(i int) {
		b := calls[i]
		out, isErr, err := s.subagentTool(ctx, rules, allowed, b, run, id, record)
		results[i], errs[i] = Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr}, err
	}
	for i := 0; i < len(calls); {
		n := s.parallelGroup(calls[i:], allowed)
		if n < 2 {
			n = 1
			runOne(i)
		} else {
			var wg sync.WaitGroup
			for j := i; j < i+n; j++ {
				wg.Add(1)
				go func(j int) { defer wg.Done(); runOne(j) }(j)
			}
			wg.Wait()
		}
		for j := i; j < i+n; j++ {
			if errs[j] != nil {
				return nil, errs[j]
			}
		}
		i += n
	}
	return results, nil
}

func toolUses(content []Block) []Block {
	var out []Block
	for _, b := range content {
		if b.Type == "tool_use" {
			out = append(out, b)
		}
	}
	return out
}
