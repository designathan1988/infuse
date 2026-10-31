package main

// The Memory tool and the session-bound scope: the store and the prompt live in internal/knowledge.

import (
	"context"
	"encoding/json"
	"infuse/internal/knowledge"
)

const memoryRel = knowledge.MemoryRel
const memoryBudget = knowledge.MemoryBudget

type MemoryItem = knowledge.MemoryItem

func projectMemoryFile(root string) string { return knowledge.ProjectMemoryFile(root) }
func globalMemoryFile() string             { return knowledge.GlobalMemoryFile() }
func loadMemory(file string) []MemoryItem  { return knowledge.LoadMemory(file) }
func saveMemory(file string, items []MemoryItem) error {
	return knowledge.SaveMemory(file, items)
}
func normalizeMemory(it *MemoryItem) error { return knowledge.NormalizeMemory(it) }

func memoryPrompt(s *Session) string {
	s.mu.Lock()
	items := append([]MemoryItem{}, s.Memory...)
	s.mu.Unlock()
	return knowledge.MemoryPrompt(s.Root, items)
}

func memoryItems(s *Session) (string, int) {
	s.mu.Lock()
	items := append([]MemoryItem{}, s.Memory...)
	s.mu.Unlock()
	return knowledge.MemoryItems(s.Root, items)
}

type memoryTool struct{}

func (memoryTool) Category() string { return "memory" }
func (memoryTool) Spec() ToolSpec {
	return ToolSpec{Name: "Memory", Description: "Records, lists or removes items of the structured memory, which comes back in every request (also after compaction). Types: correcao (the user corrected something), criterio (an agreed acceptance criterion), decisao (a decision), beco (a dead end: an approach that does not work, and why), comando (a command that worked), arquivo (a file touched, and why), fato (a fact). Scope: session, project (default) or global. Priority 1-3.",
		InputSchema: schema(`{"type":"object","properties":{"action":{"type":"string","enum":["add","remove","list"]},"type":{"type":"string","enum":["correcao","criterio","decisao","beco","comando","arquivo","fato"]},"text":{"type":"string"},"scope":{"type":"string","enum":["session","project","global"]},"priority":{"type":"integer","minimum":1,"maximum":3},"id":{"type":"string"}},"required":["action"]}`)}
}

func (memoryTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Action, Type, Text, Scope, ID string
		Priority                      int
	}
	_ = json.Unmarshal(input, &in)
	s := tc.S
	switch in.Action {
	case "list":
		items, left := memoryItems(s)
		if left > 0 {
			items += tc.Say(M("memory.overflow", "count", left)) + "\n"
		}
		if items != "" {
			return items, false
		}
		return tc.Say(M("memory.empty")), false
	case "add":
		it := MemoryItem{Type: in.Type, Text: in.Text, Priority: in.Priority, Origin: "agente", Session: s.ID}
		if err := normalizeMemory(&it); err != nil {
			return tc.SayErr(err), true
		}
		if err := s.editMemory(in.Scope, func(items []MemoryItem) []MemoryItem { return append(items, it) }); err != nil {
			return err.Error(), true
		}
		s.emit("memory", map[string]any{"action": "add", "item": it})
		return tc.Say(M("memory.recorded", "id", it.ID)), false
	case "remove":
		found := false
		for _, scope := range []string{"session", "project", "global"} {
			_ = s.editMemory(scope, func(items []MemoryItem) []MemoryItem {
				out := items[:0]
				for _, x := range items {
					if x.ID == in.ID {
						found = true
						continue
					}
					out = append(out, x)
				}
				return out
			})
		}
		if !found {
			return tc.Say(M("memory.not_found", "id", in.ID)), true
		}
		s.emit("memory", map[string]any{"action": "remove", "id": in.ID})
		return tc.Say(M("memory.removed", "id", in.ID)), false
	}
	return tc.Say(M("tool.unknown_action", "action", in.Action)), true
}

// editMemory changes the items of one scope (session, project — the default — or global).
func (s *Session) editMemory(scope string, f func([]MemoryItem) []MemoryItem) error {
	switch scope {
	case "session":
		s.mu.Lock()
		s.Memory = f(append([]MemoryItem{}, s.Memory...))
		s.mu.Unlock()
		s.save()
		return nil
	case "global":
		return saveMemory(globalMemoryFile(), f(loadMemory(globalMemoryFile())))
	default:
		return saveMemory(projectMemoryFile(s.Root), f(loadMemory(projectMemoryFile(s.Root))))
	}
}

func init() { registerTool(memoryTool{}) }
