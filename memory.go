package main

// Structured memory: typed items (decision, user correction, acceptance criterion, file touched and why,
// command that worked, dead end, fact) kept per session, per project (.infuse/memory/items.json) and for
// every project (~/.infuse/memory.json). They go into every request within a fixed budget, highest
// priority first, so they survive compaction. The agent records and removes items only through the
// Memory tool (the folder is protected from its other tools); the user curates them in the Memory pane.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const memoryRel = ".infuse/memory"

type MemoryItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // decisao | correcao | criterio | arquivo | comando | beco | fato
	Text     string `json:"text"`
	Origin   string `json:"origin"` // agente | usuario
	Session  string `json:"session,omitempty"`
	Priority int    `json:"priority"` // 1 normal · 2 importante · 3 essencial
	Created  int64  `json:"created"`
}

var memoryTypes = map[string]string{
	"correcao": "correção do usuário",
	"criterio": "critério de aceite",
	"decisao":  "decisão",
	"beco":     "beco sem saída",
	"comando":  "comando que funcionou",
	"arquivo":  "arquivo",
	"fato":     "fato",
}

// typeRank orders items of the same priority: what the user said first, trivia last.
var typeRank = map[string]int{"correcao": 0, "criterio": 1, "decisao": 2, "beco": 3, "comando": 4, "arquivo": 5, "fato": 6}

const memoryBudget = 8000 // characters (~2k tokens) of memory in each request

func projectMemoryFile(root string) string {
	return filepath.Join(root, infuseDirName, "memory", "items.json")
}
func globalMemoryFile() string { return filepath.Join(InfuseHome(), "memory.json") }

func loadMemory(file string) []MemoryItem {
	var items []MemoryItem
	_ = readJSON(file, &items)
	if items == nil {
		items = []MemoryItem{}
	}
	return items
}

func saveMemory(file string, items []MemoryItem) error {
	_ = os.MkdirAll(filepath.Dir(file), 0o755)
	return writeJSON(file, items)
}

func normalizeMemory(it *MemoryItem) error {
	it.Text = strings.TrimSpace(it.Text)
	if it.Text == "" {
		return fmt.Errorf("texto vazio")
	}
	if _, ok := memoryTypes[it.Type]; !ok {
		it.Type = "fato"
	}
	if it.Priority < 1 || it.Priority > 3 {
		it.Priority = 1
	}
	if it.ID == "" {
		it.ID = "m" + randomHex(6) // a clock-based id repeats on Windows (coarse timer)
	}
	if it.Created == 0 {
		it.Created = time.Now().UnixMilli()
	}
	if len(it.Text) > 1000 {
		it.Text = it.Text[:1000]
	}
	return nil
}

func sortMemory(items []MemoryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if typeRank[a.Type] != typeRank[b.Type] {
			return typeRank[a.Type] < typeRank[b.Type]
		}
		return a.Created > b.Created
	})
}

// memoryPrompt is the memory section of the system prompt: session, project and global items, the most
// important first, cut at the budget (what does not fit is named, so it can be listed with the tool).
func memoryPrompt(s *Session) string {
	type scoped struct {
		MemoryItem
		scope string
	}
	var all []scoped
	s.mu.Lock()
	for _, it := range s.Memory {
		all = append(all, scoped{it, "sessão"})
	}
	s.mu.Unlock()
	for _, it := range loadMemory(projectMemoryFile(s.Root)) {
		all = append(all, scoped{it, "projeto"})
	}
	for _, it := range loadMemory(globalMemoryFile()) {
		all = append(all, scoped{it, "global"})
	}
	if len(all) == 0 {
		return ""
	}
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if typeRank[a.Type] != typeRank[b.Type] {
			return typeRank[a.Type] < typeRank[b.Type]
		}
		return a.Created > b.Created
	})
	var b strings.Builder
	left := 0
	for i, it := range all {
		line := fmt.Sprintf("- [%s · %s] %s (id %s)\n", memoryTypes[it.Type], it.scope, it.Text, it.ID)
		if b.Len()+len(line) > memoryBudget {
			left = len(all) - i
			break
		}
		b.WriteString(line)
	}
	if left > 0 {
		fmt.Fprintf(&b, "(%d item(ns) de menor prioridade não couberam; liste com Memory action=list)\n", left)
	}
	return b.String()
}

// ---------------------------------------------------------------- the agent's tool

type memoryTool struct{}

func (memoryTool) Category() string { return "memory" }
func (memoryTool) Spec() ToolSpec {
	return ToolSpec{Name: "Memory", Description: "Registra, lista ou remove itens da memória estruturada, que volta em toda requisição (também depois de compactar). Tipos: correcao (o usuário corrigiu algo), criterio (critério de aceite combinado), decisao, beco (abordagem que não funciona e por quê), comando (comando que funcionou), arquivo (arquivo tocado e por quê), fato. Escopo: session, project (padrão) ou global. Prioridade 1-3.",
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
		if p := memoryPrompt(s); p != "" {
			return p, false
		}
		return "memória vazia", false
	case "add":
		it := MemoryItem{Type: in.Type, Text: in.Text, Priority: in.Priority, Origin: "agente", Session: s.ID}
		if err := normalizeMemory(&it); err != nil {
			return err.Error(), true
		}
		if err := s.editMemory(in.Scope, func(items []MemoryItem) []MemoryItem { return append(items, it) }); err != nil {
			return err.Error(), true
		}
		s.emit("memory", map[string]any{"action": "add", "item": it})
		return "registrado: " + it.ID, false
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
			return "item não encontrado: " + in.ID, true
		}
		s.emit("memory", map[string]any{"action": "remove", "id": in.ID})
		return "removido: " + in.ID, false
	}
	return "ação desconhecida: " + in.Action, true
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
