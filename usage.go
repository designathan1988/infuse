package main

// Usage ledger: one line per model response in ~/.infuse/usage.jsonl, so cost can be totalled per
// session, project and day (the session file only keeps its own running total).

import (
	"path/filepath"
	"sort"
	"time"
)

type usageEntry struct {
	Ts       int64   `json:"ts"`
	Root     string  `json:"root"`
	Session  string  `json:"session"`
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Input    int     `json:"input"`
	Output   int     `json:"output"`
	Cache    int     `json:"cache"`
	Cost     float64 `json:"cost"`
}

func usageFile() string { return filepath.Join(InfuseHome(), "usage.jsonl") }

func recordUsage(e usageEntry) {
	e.Ts = time.Now().UnixMilli()
	appendLine(usageFile(), e)
}

type usageTotals struct {
	Today     float64            `json:"today"`
	Month     float64            `json:"month"`
	ByDay     []dayTotal         `json:"byDay"`
	ByProject map[string]float64 `json:"byProject"`
	ByModel   map[string]float64 `json:"byModel"`
	Tokens    struct {
		Input  int `json:"input"`
		Output int `json:"output"`
	} `json:"tokens"`
}

type dayTotal struct {
	Day  string  `json:"day"`
	Cost float64 `json:"cost"`
}

func UsageTotals() usageTotals {
	t := usageTotals{ByProject: map[string]float64{}, ByModel: map[string]float64{}, ByDay: []dayTotal{}}
	now := time.Now()
	today := now.Format("2006-01-02")
	month := now.Format("2006-01")
	days := map[string]float64{}
	for _, e := range readJSONL(usageFile()) {
		ts, _ := e["ts"].(float64)
		cost, _ := e["cost"].(float64)
		day := time.UnixMilli(int64(ts)).Format("2006-01-02")
		days[day] += cost
		if day == today {
			t.Today += cost
		}
		if day[:7] == month {
			t.Month += cost
		}
		if r, _ := e["root"].(string); r != "" {
			t.ByProject[r] += cost
		}
		if m, _ := e["model"].(string); m != "" {
			t.ByModel[m] += cost
		}
		in, _ := e["input"].(float64)
		out, _ := e["output"].(float64)
		t.Tokens.Input += int(in)
		t.Tokens.Output += int(out)
	}
	for d, c := range days {
		t.ByDay = append(t.ByDay, dayTotal{d, c})
	}
	sort.Slice(t.ByDay, func(i, j int) bool { return t.ByDay[i].Day > t.ByDay[j].Day })
	if len(t.ByDay) > 30 {
		t.ByDay = t.ByDay[:30]
	}
	return t
}
