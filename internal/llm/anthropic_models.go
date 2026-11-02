package llm

// What each Claude family accepts on the Messages API, as a table: no request code asks which model it
// is talking to. Source: Anthropic's API reference as bundled with the claude-api skill (2026-10):
//   - budget_tokens is a 400 on every model from Opus 4.7 on, and on Sonnet 5 and Haiku 5.5;
//   - forced tool_choice (any, tool) is a 400 on Fable 5.1, Mythos 5.1, Opus 5.5 and Sonnet 5.5;
//   - {"type":"disabled"} is a 400 on Fable, Opus 5.5 and Sonnet 5.5 (which has "between_tools" instead);
//     on Opus 5 it holds only at effort high or below and has known failure modes, so effort is the
//     control there too;
//   - thinking text is omitted unless display is "summarized" on everything from Opus 4.7 on;
//   - Fable 5.1, Opus 5.5, Sonnet 5.5 and Haiku 5.5 run the history-editing check on replayed blocks.

import "strings"

type claudeFamily struct {
	prefix    string
	thinking  string   // how reasoning is asked for: adaptive | enabled (budget_tokens)
	display   string   // thinking.display to ask for, so the reasoning can be shown
	off       string   // the thinking.type that turns reasoning off; "" = leave the field out
	alwaysOn  bool     // reasoning cannot be turned off: only effort lowers it
	efforts   []string // what output_config.effort accepts; empty = the field is rejected
	forceTool bool     // tool_choice any/tool is accepted
	binding   bool     // thinking.block_binding applies (the history-editing check)
}

var (
	effortAll     = []string{"low", "medium", "high", "xhigh", "max"}
	effortNoXHigh = []string{"low", "medium", "high", "max"}
	effortToHigh  = []string{"low", "medium", "high"}
)

var claudeFamilies = []claudeFamily{
	{prefix: "claude-fable-5-1", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, binding: true},
	{prefix: "claude-mythos-5-1", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll},
	{prefix: "claude-fable-5", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, forceTool: true},
	{prefix: "claude-mythos-5", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, forceTool: true},
	{prefix: "claude-opus-5-5", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, binding: true},
	{prefix: "claude-opus-5", thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, forceTool: true},
	{prefix: "claude-opus-4-8", thinking: "adaptive", display: "summarized", efforts: effortAll, forceTool: true},
	{prefix: "claude-opus-4-7", thinking: "adaptive", display: "summarized", efforts: effortAll, forceTool: true},
	{prefix: "claude-opus-4-6", thinking: "adaptive", efforts: effortNoXHigh, forceTool: true},
	{prefix: "claude-opus-4-5", thinking: "enabled", efforts: effortToHigh, forceTool: true},
	{prefix: "claude-sonnet-5-5", thinking: "adaptive", display: "summarized", off: "between_tools", efforts: effortAll, binding: true},
	{prefix: "claude-sonnet-5", thinking: "adaptive", display: "summarized", off: "disabled", efforts: effortAll, forceTool: true},
	{prefix: "claude-sonnet-4-6", thinking: "adaptive", efforts: effortNoXHigh, forceTool: true},
	{prefix: "claude-sonnet-4-5", thinking: "enabled", forceTool: true},
	{prefix: "claude-haiku-5-5", thinking: "adaptive", display: "summarized", off: "disabled", efforts: effortAll, forceTool: true, binding: true},
	{prefix: "claude-haiku-4-5", thinking: "enabled", forceTool: true},
	{prefix: "claude-opus-4-0", thinking: "enabled", forceTool: true},
	{prefix: "claude-opus-4-1", thinking: "enabled", forceTool: true},
	{prefix: "claude-opus-4-2", thinking: "enabled", forceTool: true}, // dated id: claude-opus-4-20250514
	{prefix: "claude-opus-4@", thinking: "enabled", forceTool: true},
	{prefix: "claude-sonnet-4-0", thinking: "enabled", forceTool: true},
	{prefix: "claude-sonnet-4-2", thinking: "enabled", forceTool: true}, // dated id: claude-sonnet-4-20250514
	{prefix: "claude-sonnet-4@", thinking: "enabled", forceTool: true},
	{prefix: "claude-3", thinking: "enabled", forceTool: true},
}

var (
	// newestClaude is a Claude model newer than this table: the strictest current behaviour.
	newestClaude = claudeFamily{thinking: "adaptive", display: "summarized", alwaysOn: true, efforts: effortAll, binding: true}
	// otherModel is anything else served through a Messages-compatible endpoint.
	otherModel = claudeFamily{thinking: "enabled", forceTool: true}
)

// familyOf finds a model's row by its longest prefix, whatever the platform wraps around the id
// (anthropic.claude-…, us.anthropic.claude-…, claude-…@date).
func familyOf(model string) claudeFamily {
	m := strings.ToLower(strings.TrimSpace(model))
	i := strings.Index(m, "claude-")
	if i < 0 {
		return otherModel
	}
	m = m[i:]
	best, found := claudeFamily{}, false
	for _, f := range claudeFamilies {
		if strings.HasPrefix(m, f.prefix) && len(f.prefix) > len(best.prefix) {
			best, found = f, true
		}
	}
	if !found {
		return newestClaude
	}
	return best
}

// effortOf is the output_config.effort to send: what was asked, or the nearest level below it the
// family accepts. When reasoning cannot be turned off, a request that wants none runs at the lowest.
func (f claudeFamily) effortOf(req ModelRequest) string {
	want := req.Effort
	if want == "" && f.alwaysOn && req.Thinking <= 0 {
		want = "low"
	}
	rank := -1
	for i, e := range effortAll {
		if e == want {
			rank = i
		}
	}
	for ; rank >= 0; rank-- {
		for _, e := range f.efforts {
			if e == effortAll[rank] {
				return e
			}
		}
	}
	return ""
}
