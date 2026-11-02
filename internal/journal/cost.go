package journal

// Perception read from the journal: what each command costs, and which files the current work has
// already touched.

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// SlowCommandMillis is what counts as slow: a command an agent should not repeat without a reason.
const SlowCommandMillis = 10_000

// costWindow is how far back the cost of a command is measured.
const costWindow = 14 * 24 * time.Hour

// CommandCost is what one command has cost so far.
type CommandCost struct {
	Command string `json:"command"`
	Runs    int    `json:"runs"`
	Median  int64  `json:"medianMs"`
	Max     int64  `json:"maxMs"`
}

// RecordCost journals how long a command took.
func RecordCost(root, command string, millis int64) {
	Journal(root, JournalEntry{Kind: "cost", Command: command, Millis: millis})
}

// commandKey folds a command to what makes it the same command again.
func commandKey(cmd string) string { return strings.Join(strings.Fields(cmd), " ") }

// Costs measures the commands of the recent journal, slowest first.
func Costs(root string) []CommandCost {
	byCommand := map[string][]int64{}
	for _, e := range ReadJournalSince(root, time.Now().Add(-costWindow).UnixMilli()) {
		if e.Kind == "cost" && e.Command != "" {
			k := commandKey(e.Command)
			byCommand[k] = append(byCommand[k], e.Millis)
		}
	}
	out := make([]CommandCost, 0, len(byCommand))
	for cmd, runs := range byCommand {
		sort.Slice(runs, func(i, j int) bool { return runs[i] < runs[j] })
		out = append(out, CommandCost{Command: cmd, Runs: len(runs), Median: runs[len(runs)/2], Max: runs[len(runs)-1]})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Median != out[j].Median {
			return out[i].Median > out[j].Median
		}
		return out[i].Command < out[j].Command
	})
	return out
}

// SlowCommands lists up to n commands whose typical run is slow, for the agent's prompt.
func SlowCommands(root string, n int) []CommandCost {
	var out []CommandCost
	for _, c := range Costs(root) {
		if c.Median < SlowCommandMillis || len(out) == n {
			break
		}
		out = append(out, c)
	}
	return out
}

// CostOf is the recorded cost of one command, when it ran before.
func CostOf(root, command string) (CommandCost, bool) {
	k := commandKey(command)
	for _, c := range Costs(root) {
		if c.Command == k {
			return c, true
		}
	}
	return CommandCost{}, false
}

// CostLine is one measured command, as the agent reads it.
func (c CommandCost) CostLine() string {
	return fmt.Sprintf("%s: %s typical, %s at most, %d run(s)", c.Command, seconds(c.Median), seconds(c.Max), c.Runs)
}

func seconds(ms int64) string { return fmt.Sprintf("%.1fs", float64(ms)/1000) }

// touchedWindow is how long a write counts as part of the current work. An external agent has no
// session infuse can see, so the journal's recent writes stand for it.
const touchedWindow = 2 * time.Hour

// TouchedFiles lists the files written recently, as a set.
func TouchedFiles(root string) map[string]bool {
	out := map[string]bool{}
	for _, e := range ReadJournalSince(root, time.Now().Add(-touchedWindow).UnixMilli()) {
		if e.Kind == "write" && e.Path != "" {
			out[e.Path] = true
		}
	}
	return out
}

// LastWrite is when the project last changed through the filter (0 when unknown).
func LastWrite(root string) int64 {
	last := int64(0)
	for _, e := range ReadJournalSince(root, time.Now().Add(-touchedWindow).UnixMilli()) {
		if e.Kind == "write" && e.Ts > last {
			last = e.Ts
		}
	}
	return last
}
