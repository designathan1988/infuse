package config

// What a model response costs. Settings.Pricing holds the list prices (the user may edit them); how each
// provider bills cached tokens and long prompts is the table below, read from the providers' price pages.
//   Anthropic: the claude-api skill's reference (2026-10): cache reads 0.1x (Fable 5.1 and Mythos 5.1
//   0.025x, Opus 5.5 0.05x), 5-minute writes 1.25x; Haiku 5.5 is $0.50/$2.50 above 100K prompt tokens.
//   Google: https://ai.google.dev/gemini-api/docs/pricing (read 2026-11-02).
//   DeepSeek: https://api-docs.deepseek.com/quick_start/pricing (read 2026-11-02); the prices are the peak
//   ones, off-peak is half, so a DeepSeek cost is an upper bound.
//   OpenAI: last published ratios of cached to full input; its page refused the read on 2026-11-02.
// A provider with no row gets no discount and no premium: an estimate that never undercounts.

import (
	"infuse/internal/core"
	"strings"
)

type billingRule struct {
	provider   string
	prefix     string
	cacheRead  float64 // multiples of the input price
	cacheWrite float64
	longAbove  int // prompt tokens above which the long-prompt prices apply; 0 = none
	longInput  float64
	longOutput float64
}

var billingRules = []billingRule{
	{provider: "anthropic", prefix: "claude-", cacheRead: 0.1, cacheWrite: 1.25},
	{provider: "anthropic", prefix: "claude-fable-5-1", cacheRead: 0.025, cacheWrite: 1.25},
	{provider: "anthropic", prefix: "claude-mythos-5-1", cacheRead: 0.025, cacheWrite: 1.25},
	{provider: "anthropic", prefix: "claude-opus-5-5", cacheRead: 0.05, cacheWrite: 1.25},
	{provider: "anthropic", prefix: "claude-haiku-5-5", cacheRead: 0.1, cacheWrite: 1.25, longAbove: 100_000, longInput: 5, longOutput: 5},
	{provider: "openai", prefix: "gpt-5", cacheRead: 0.1, cacheWrite: 1},
	{provider: "openai", prefix: "gpt-4.1", cacheRead: 0.25, cacheWrite: 1},
	{provider: "openai", prefix: "gpt-4o", cacheRead: 0.5, cacheWrite: 1},
	{provider: "openai", prefix: "o3", cacheRead: 0.25, cacheWrite: 1},
	{provider: "google", prefix: "gemini-2.5-flash", cacheRead: 0.1, cacheWrite: 1},
	{provider: "google", prefix: "gemini-2.5-pro", cacheRead: 0.1, cacheWrite: 1, longAbove: 200_000, longInput: 2, longOutput: 1.5},
	{provider: "deepseek", prefix: "deepseek-", cacheRead: 0.02, cacheWrite: 1},
	{provider: "deepseek", prefix: "deepseek-v4-pro", cacheRead: 1.0 / 30, cacheWrite: 1},
}

// billingOf is the row with the longest prefix, or full price for cached tokens.
func billingOf(model string) billingRule {
	best := billingRule{cacheRead: 1, cacheWrite: 1}
	for _, r := range billingRules {
		if strings.HasPrefix(model, r.prefix) && len(r.prefix) > len(best.prefix) {
			best = r
		}
	}
	return best
}

// bareModel is the id without what a router or a cloud wraps around it: anthropic/claude-…,
// anthropic.claude-…, us.anthropic.claude-…
func bareModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndex(m, "/"); i >= 0 {
		m = m[i+1:]
	}
	if i := strings.Index(m, "anthropic."); i >= 0 {
		m = m[i+len("anthropic."):]
	}
	return m
}

func (s Settings) priceFor(model string) (float64, float64) {
	model = bareModel(model)
	best := ""
	for prefix := range s.Pricing {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best = prefix
		}
	}
	if best == "" {
		return 0, 0
	}
	return s.Pricing[best][0], s.Pricing[best][1]
}

// HasPrice reports whether the model's price is known (an unknown price counts as zero cost).
func (s Settings) HasPrice(model string) bool { in, out := s.priceFor(model); return in > 0 || out > 0 }

// Priced reports whether the cost of a model on a connection is known: local models cost nothing, a
// paid model with no price has a cost nobody can state, which must show instead of reading as $0.
func (s Settings) Priced(connection, model string) bool {
	return PresetFor(connection).Local || s.HasPrice(model)
}

func (s Settings) CostOf(model string, u core.Usage) float64 {
	in, out := s.priceFor(model)
	r := billingOf(bareModel(model))
	if r.longAbove > 0 && u.Input+u.CacheRead+u.CacheWrite > r.longAbove {
		in, out = in*r.longInput, out*r.longOutput
	}
	return (float64(u.Input)+float64(u.CacheWrite)*r.cacheWrite+float64(u.CacheRead)*r.cacheRead)*in/1e6 + float64(u.Output)*out/1e6
}
