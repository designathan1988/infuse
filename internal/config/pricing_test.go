package config

import (
	"infuse/internal/core"
	"math"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

const mtok = 1_000_000

// One million tokens of each kind, at the published rates of each family.
func TestPricesOfTheCurrentFamilies(t *testing.T) {
	s := DefaultSettings()
	cases := []struct {
		model                  string
		in, out, read, write5m float64
	}{
		{"claude-fable-5-1", 10, 50, 0.25, 12.5},
		{"claude-fable-5", 10, 50, 1, 12.5},
		{"claude-opus-5-5", 4, 20, 0.2, 5},
		{"claude-opus-5", 5, 25, 0.5, 6.25},
		{"claude-opus-4-8", 5, 25, 0.5, 6.25},
		{"claude-sonnet-5-5", 2, 10, 0.2, 2.5},
		{"claude-sonnet-5", 2, 10, 0.2, 2.5},
		{"claude-sonnet-4-6", 3, 15, 0.3, 3.75},
		{"claude-haiku-4-5-20251001", 1, 5, 0.1, 1.25},
		{"anthropic.claude-opus-5-5", 4, 20, 0.2, 5},
		{"us.anthropic.claude-sonnet-5-5", 2, 10, 0.2, 2.5},
		{"anthropic/claude-sonnet-5-5", 2, 10, 0.2, 2.5},
		{"gemini-2.5-flash", 0.3, 2.5, 0.03, 0.3},
		{"deepseek-flash", 0.3, 1.2, 0.006, 0.3},
		{"deepseek-v4-pro", 1.32, 3.96, 0.044, 1.32},
	}
	for _, c := range cases {
		parts := []struct {
			u    core.Usage
			want float64
		}{
			{core.Usage{Input: mtok / 10}, c.in / 10},
			{core.Usage{Output: mtok}, c.out},
			{core.Usage{CacheRead: mtok / 10}, c.read / 10},
			{core.Usage{CacheWrite: mtok / 10}, c.write5m / 10},
		}
		for _, p := range parts {
			if got := s.CostOf(c.model, p.u); !near(got, p.want) {
				t.Errorf("%s %+v: %v, want %v", c.model, p.u, got, p.want)
			}
		}
	}
}

func TestLongPromptPrices(t *testing.T) {
	s := DefaultSettings()
	if got := s.CostOf("claude-haiku-5-5", core.Usage{Input: 100_000, Output: mtok}); !near(got, 0.01+0.5) {
		t.Fatalf("at the limit: %v", got)
	}
	if got := s.CostOf("claude-haiku-5-5", core.Usage{Input: 100_000, CacheRead: 1, Output: mtok}); !near(got, 0.05+0.00000005+2.5) {
		t.Fatalf("above it, cached tokens counted in the prompt: %v", got)
	}
	if got := s.CostOf("gemini-2.5-pro", core.Usage{Input: 300_000, Output: mtok}); !near(got, 0.75+15) {
		t.Fatalf("gemini: %v", got)
	}
}

// A provider with no billing row pays the full input price for cached tokens: never an undercount.
func TestCacheMultipliersArePerProvider(t *testing.T) {
	s := DefaultSettings()
	s.Pricing["acme-1"] = [2]float64{10, 10}
	if got := s.CostOf("acme-1", core.Usage{CacheRead: mtok, CacheWrite: mtok}); !near(got, 20) {
		t.Fatalf("unknown provider: %v", got)
	}
	if got := s.CostOf("gpt-5", core.Usage{CacheRead: mtok, CacheWrite: mtok}); !near(got, 0.125+1.25) {
		t.Fatalf("openai has no write premium: %v", got)
	}
}

func TestStaleSettingsFileGetsCurrentPrices(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	_ = core.WriteJSON(SettingsFile(), map[string]any{"pricing": map[string][2]float64{"claude-opus": {15, 75}, "claude-fable": {15, 75}}})
	s := LoadSettings()
	if got := s.CostOf("claude-opus-5-5", core.Usage{Input: mtok}); !near(got, 4) {
		t.Fatalf("opus 5.5: %v", got)
	}
	if got := s.CostOf("claude-fable-5-1", core.Usage{Input: mtok}); !near(got, 10) {
		t.Fatalf("fable 5.1: %v", got)
	}
}

func TestUnpricedIsNotFree(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	s := DefaultSettings()
	if s.Priced("openrouter", "acme/secret-model") {
		t.Fatal("a paid model with no price reads as priced")
	}
	if !s.Priced("ollama", "llama3") || !s.Priced("mock", "mock") {
		t.Fatal("a local model is free, not unpriced")
	}
	RecordUsage(UsageEntry{Model: "acme/secret-model", Input: 5, Unpriced: true})
	RecordUsage(UsageEntry{Model: "claude-opus-5-5", Input: 5, Cost: 1})
	if u := UsageTotals().Unpriced; len(u) != 1 || u[0] != "acme/secret-model" {
		t.Fatalf("%v", u)
	}
}
