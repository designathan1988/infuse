package agent

import (
	"context"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/llm"
	"strings"
	"testing"
	"time"
)

// spendProvider answers everything with fixed usage and remembers the models it was asked for.
type spendProvider struct {
	usage  Usage
	models []string
}

func (p *spendProvider) Name() string { return "spend" }
func (p *spendProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	p.models = append(p.models, req.Model)
	if req.ForceTool != "" {
		return ModelResult{Content: []Block{{Type: "tool_use", ID: "t", Name: req.ForceTool, Input: json.RawMessage(`{}`)}}, StopReason: "tool_use", Usage: p.usage}, nil
	}
	return ModelResult{Content: []Block{{Type: "text", Text: "summary"}}, StopReason: "end_turn", Usage: p.usage}, nil
}

func spendSession(t *testing.T, name string, p Provider) *Session {
	t.Helper()
	testHome(t)
	llm.RegisterProvider(name, func(llm.ProviderConfig) Provider { return p })
	s := newSession(t.TempDir())
	s.settings = config.DefaultSettings()
	s.settings.Provider = name
	s.settings.Connections = map[string]config.Connection{name: {Kind: name, Connected: true, Models: []string{"claude-opus-5-5", "claude-haiku-5-5"}}}
	s.settings.DailyLimitUSD = 0
	s.Provider, s.Model, s.Turn = name, "claude-opus-5-5", 1
	return s
}

func ledger(t *testing.T) (lines int, cost float64) {
	t.Helper()
	totals := config.UsageTotals()
	for _, c := range totals.ByModel {
		cost += c
	}
	return totals.Tokens.Input, cost
}

// Every kind of model call reaches the session's total, the turn's and the ledger: they agree.
func TestEveryModelCallIsInTheLedger(t *testing.T) {
	p := &spendProvider{usage: Usage{Input: 1000, Output: 100, CacheRead: 10, CacheWrite: 5}}
	s := spendSession(t, "spend-all", p)
	ctx := context.Background()
	s.Messages = []Message{
		{Role: "user", Content: []Block{{Type: "text", Text: "do not touch vendor/"}}, TS: 1},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "ok"}}, TS: 2},
		{Role: "user", Content: []Block{{Type: "text", Text: "go"}}, TS: 3},
		{Role: "assistant", Content: []Block{{Type: "text", Text: "done"}}, TS: 4},
	}
	s.TurnStarts = []int{0}
	calls := []func(){
		func() { s.compileBans(ctx, p) },
		func() { _, _ = s.compileInstructions(ctx, p, "always run the tests") },
		func() { _, _ = s.digestDocument(ctx, p, "a.md", []byte("one line")) },
		func() { _ = s.compact(ctx, p) },
	}
	for i, call := range calls {
		call()
		if tokens, _ := ledger(t); tokens != (i+1)*1000 {
			t.Fatalf("call %d: %d input tokens in the ledger", i, tokens)
		}
	}
	if _, err := s.SideQuestion(ctx, "why?"); err != nil {
		t.Fatal(err)
	}
	if out := s.auditTurn(ctx); out == "" {
		t.Fatal("audit did not run")
	}
	tokens, cost := ledger(t)
	if tokens != 6000 {
		t.Fatalf("%d input tokens in the ledger", tokens)
	}
	if cost <= 0 || !sameCost(s.Cost, cost) || !sameCost(s.TurnCosts[0], cost) {
		t.Fatalf("session %v, turn %v, ledger %v", s.Cost, s.TurnCosts, cost)
	}
	if s.Usage.CacheRead != 60 || s.Usage.CacheWrite != 30 {
		t.Fatalf("usage %+v", s.Usage)
	}
}

func sameCost(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

func TestTurnCostInTheSnapshot(t *testing.T) {
	s := spendSession(t, "spend-turn", &spendProvider{})
	s.spend("claude-opus-5-5", Usage{Input: 1_000_000})
	s.Turn = 2
	s.spend("claude-haiku-5-5", Usage{Output: 1_000_000})
	snap := s.snapshot()
	if !sameCost(snap["turnCost"].(float64), 0.5) || len(snap["turnCosts"].([]float64)) != 2 || !sameCost(snap["cost"].(float64), 4.5) {
		t.Fatalf("%v %v %v", snap["turnCost"], snap["turnCosts"], snap["cost"])
	}
}

func TestUnpricedModelShowsOnceAndIsMarkedInTheLedger(t *testing.T) {
	s := spendSession(t, "spend-unpriced", &spendProvider{})
	ch, cancel := s.subscribe()
	defer cancel()
	s.spend("acme-model", Usage{Input: 10})
	s.spend("acme-model", Usage{Input: 10})
	if len(s.Unpriced) != 1 || s.Cost != 0 {
		t.Fatalf("%v %v", s.Unpriced, s.Cost)
	}
	if u := config.UsageTotals().Unpriced; len(u) != 1 || u[0] != "acme-model" {
		t.Fatalf("ledger: %v", u)
	}
	gates := 0
	for {
		select {
		case ev := <-ch:
			if ev.Type == "gate" {
				gates++
			}
			continue
		case <-time.After(50 * time.Millisecond):
		}
		break
	}
	if gates != 1 {
		t.Fatalf("%d notices", gates)
	}
}

func TestDailyLimitWarnsThenStops(t *testing.T) {
	s := spendSession(t, "spend-limit", &spendProvider{})
	s.settings.DailyLimitUSD = 10
	ch, cancel := s.subscribe()
	defer cancel()
	s.spend("claude-opus-5-5", Usage{Input: 500_000}) // $2
	if s.warnedDay != "" {
		t.Fatal("warned too early")
	}
	s.spend("claude-opus-5-5", Usage{Input: 1_600_000}) // $8.40 in all
	s.spend("claude-opus-5-5", Usage{Input: 100_000})   // still below the limit: one warning is enough
	warnings := 0
	for done := false; !done; {
		select {
		case ev := <-ch:
			if b, _ := json.Marshal(ev.Data); ev.Type == "gate" && strings.Contains(string(b), "8.") {
				warnings++
			}
		case <-time.After(50 * time.Millisecond):
			done = true
		}
	}
	if warnings != 1 {
		t.Fatalf("%d warnings", warnings)
	}
	if _, over := s.overDailyLimit(); over {
		t.Fatal("stopped before the limit")
	}
	s.spend("claude-opus-5-5", Usage{Input: 1_000_000})
	if stop, over := s.overDailyLimit(); !over || stop.ID != "session.daily_limit" {
		t.Fatalf("%v %v", stop, over)
	}
}

// The limit stops a subagent and a mission's next turn too, not only the main loop.
func TestDailyLimitStopsSubagentsAndMissions(t *testing.T) {
	p := &spendProvider{usage: Usage{Input: 10}}
	s := spendSession(t, "spend-over", p)
	s.settings.DailyLimitUSD = 1
	s.spend("claude-opus-5-5", Usage{Input: 1_000_000})
	if _, err := s.RunSubagent(context.Background(), "explore", "x", "task"); err == nil || !strings.Contains(err.Error(), "daily") {
		t.Fatalf("subagent: %v", err)
	}
	if len(p.models) != 0 {
		t.Fatalf("the model was called: %v", p.models)
	}
	s.Mission = &Mission{Goal: "g", BudgetUSD: 100, MaxTurns: 10, State: "running", StartCost: s.Cost, lastCostAt: s.Cost}
	s.lastOracleRun, s.gate.oracleKey, s.gate.oracleVerdict = "r", s.oracleRunKey(), "reprovado"
	s.missionStep()
	if m := s.missionSnapshot(); m.State != "orcamento" || !strings.Contains(m.Reason, "US$") {
		t.Fatalf("%+v", m)
	}
	if _, err := s.SideQuestion(context.Background(), "q"); err == nil {
		t.Fatal("side question over the limit")
	}
}

func TestSubagentsUseTheFastModelTheReviewerTheMain(t *testing.T) {
	p := &spendProvider{usage: Usage{Input: 10}}
	s := spendSession(t, "spend-model", p)
	s.settings.FastModel = "claude-haiku-5-5"
	for _, kind := range []string{"explore", "general"} {
		if _, err := s.RunSubagent(context.Background(), kind, "x", "task"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.RunSubagent(context.Background(), "review", "x", "task"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.models, " "); got != "claude-haiku-5-5 claude-haiku-5-5 claude-opus-5-5" {
		t.Fatal(got)
	}
	if s.Cost <= 0 {
		t.Fatal("subagent cost not recorded")
	}
}

// The base is what the provider measured; only what came after the request is estimated.
func TestContextTokensUseTheProviderMeasure(t *testing.T) {
	s := spendSession(t, "spend-ctx", &spendProvider{})
	big := strings.Repeat("x", 40_000)
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: big}}, TS: 7}}
	estimate := s.contextTokensLocked()
	if estimate < 10_000 {
		t.Fatalf("no measurement yet: %d", estimate)
	}
	s.recordReply(ModelResult{Content: []Block{{Type: "text", Text: "ok"}}, Usage: Usage{Input: 5, CacheRead: 300_000, CacheWrite: 20}}, time.Now())
	got := s.contextTokensLocked()
	if got < 300_025 || got > 300_100 {
		t.Fatalf("measured base plus the small reply: %d", got)
	}
	s.Messages = append(s.Messages, Message{Role: "user", Content: []Block{{Type: "text", Text: strings.Repeat("y", 4000)}}})
	if after := s.contextTokensLocked(); after < got+1000 || after > got+1100 {
		t.Fatalf("delta by characters: %d -> %d", got, after)
	}
	// the transcript changed under the measure (a compaction, a rewind): back to the estimate
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "small"}}, TS: 9}, {Role: "assistant", Content: []Block{{Type: "text", Text: "x"}}, TS: 10}, {Role: "user", Content: []Block{{Type: "text", Text: "y"}}, TS: 11}}
	if after := s.contextTokensLocked(); after > 1000 {
		t.Fatalf("stale measure used: %d", after)
	}
}

func TestNeedsCompactionFromTheMeasure(t *testing.T) {
	s := spendSession(t, "spend-need", &spendProvider{})
	s.settings.ContextLimit = 100_000
	s.Messages = []Message{{Role: "user", Content: []Block{{Type: "text", Text: "short"}}, TS: 1}}
	if s.needsCompaction() {
		t.Fatal("short conversation")
	}
	s.recordReply(ModelResult{Content: []Block{{Type: "text", Text: "ok"}}, Usage: Usage{Input: 80_000}}, time.Now())
	if !s.needsCompaction() {
		t.Fatal("the system prompt and tools the provider measured count")
	}
}

// The end of a turn tells the interface its accounts and its outcome: the badge must not need a reload.
func TestDonePayloadCarriesTheOutcomeAndTheAccounts(t *testing.T) {
	s := spendSession(t, "spend-done", &spendProvider{})
	s.spend("no-price-model", Usage{Input: 10, Output: 10})
	s.setOutcome("unverified", M("turn.unverified"))
	s.Mu.Lock()
	done := s.donePayloadLocked("")
	s.Mu.Unlock()
	if done["outcome"] != "unverified" || len(done["turnCosts"].([]float64)) != 1 || len(done["unpriced"].([]string)) != 1 {
		t.Fatalf("%+v", done)
	}
}
