package knowledge

// Structured memory: typed items (decision, user correction, acceptance criterion, file touched and why,
// command that worked, dead end, fact) kept per session, per project (.infuse/memory/items.json) and for
// every project (~/.infuse/memory.json). They go into every request within a fixed budget, highest
// priority first, so they survive compaction.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/store"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const MemoryRel = ".infuse/memory"

type MemoryItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"` // decisao | correcao | criterio | arquivo | comando | beco | fato
	Text     string `json:"text"`
	Origin   string `json:"origin"` // agente | usuario
	Session  string `json:"session,omitempty"`
	Priority int    `json:"priority"` // 1 normal · 2 importante · 3 essencial
	Created  int64  `json:"created"`
}

var MemoryTypes = map[string]string{
	"correcao": "user correction",
	"criterio": "acceptance criterion",
	"decisao":  "decision",
	"beco":     "dead end",
	"comando":  "command that worked",
	"arquivo":  "file",
	"fato":     "fact",
}

// typeRank orders items of the same priority: what the user said first, trivia last.
var typeRank = map[string]int{"correcao": 0, "criterio": 1, "decisao": 2, "beco": 3, "comando": 4, "arquivo": 5, "fato": 6}

const MemoryBudget = 8000 // characters (~2k tokens) of memory in each request

func ProjectMemoryFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "memory", "items.json")
}

func GlobalMemoryFile() string { return filepath.Join(core.InfuseHome(), "memory.json") }

func LoadMemory(file string) []MemoryItem {
	list, err := store.LoadList[MemoryItem](file)
	if err != nil || list.Items == nil {
		return []MemoryItem{}
	}
	return list.Items
}

func SaveMemory(file string, items []MemoryItem) error {
	if items == nil {
		items = []MemoryItem{}
	}
	return store.SaveList(file, store.List[MemoryItem]{Items: items})
}

func NormalizeMemory(it *MemoryItem) error {
	it.Text = strings.TrimSpace(it.Text)
	if it.Text == "" {
		return core.ErrMsg(core.M("memory.empty_text"))
	}
	if _, ok := MemoryTypes[it.Type]; !ok {
		it.Type = "fato"
	}
	if it.Priority < 1 || it.Priority > 3 {
		it.Priority = 1
	}
	if it.ID == "" {
		it.ID = "m" + core.RandomHex(6) // a clock-based id repeats on Windows (coarse timer)
	}
	if it.Created == 0 {
		it.Created = time.Now().UnixMilli()
	}
	if len(it.Text) > 1000 {
		it.Text = it.Text[:1000]
	}
	return nil
}

func sortItems(items []MemoryItem) {
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

// MemoryPrompt is the memory section of the system prompt: session, project and global items, the most
// important first, cut at the budget (what does not fit is named, so it can be listed with the tool).
func MemoryPrompt(root string, sessionItems []MemoryItem) string {
	items, left := MemoryItems(root, sessionItems)
	if left > 0 {
		items += core.M("memory.overflow", "count", left).String() + "\n"
	}
	return items
}

// MemoryItems is the memory's lines that fit the budget, and how many did not fit.
func MemoryItems(root string, sessionItems []MemoryItem) (string, int) {
	type scoped struct {
		MemoryItem
		scope string
	}
	var all []scoped
	for _, it := range sessionItems {
		all = append(all, scoped{it, "session"})
	}
	for _, it := range LoadMemory(ProjectMemoryFile(root)) {
		all = append(all, scoped{it, "project"})
	}
	for _, it := range LoadMemory(GlobalMemoryFile()) {
		all = append(all, scoped{it, "global"})
	}
	if len(all) == 0 {
		return "", 0
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
		line := fmt.Sprintf("- [%s · %s] %s (id %s)\n", MemoryTypes[it.Type], it.scope, it.Text, it.ID)
		if b.Len()+len(line) > MemoryBudget {
			left = len(all) - i
			break
		}
		b.WriteString(line)
	}
	return b.String(), left
}

// SortItems orders items by priority, kind and recency.
func SortItems(items []MemoryItem) { sortItems(items) }
