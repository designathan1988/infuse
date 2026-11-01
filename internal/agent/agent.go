package agent

// Subagents: the Agent tool runs a task in a fresh conversation (no history of the session) and returns
// only its conclusion. Every tool call of a subagent goes through the session's permissions, the filter
// and the guard, like the agent's own. Kinds:
//   explore — read-only search and reading across the project;
//   review  — independent reviewer: sees the diff, the plan and the criteria, never the agent's reasoning;
//   general — any tool except Agent itself.

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/mcp"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var readOnlyTools = map[string]bool{"Read": true, "Glob": true, "Grep": true, "LS": true, "WebFetch": true, "WebSearch": true}

func subagentAllows(kind, tool string) bool {
	switch kind {
	case "explore", "review":
		return readOnlyTools[tool]
	default:
		return tool != "Agent"
	}
}

// RunSubagent runs task in a fresh conversation and returns the final text.
func (s *Session) RunSubagent(ctx context.Context, kind, title, task string) (string, error) {
	provider, rules, specs, allowed, err := s.subagentSetup(ctx, kind)
	if err != nil {
		return "", err
	}
	if kind == "" {
		kind = "general"
	}
	id := "a" + randomHex(6)
	run := s.agentStart(id, kind, title, task)
	defer s.agentUpdate(run, func(a *AgentRun) { // an error or the step limit: the run shows as failed
		if a.State == "running" {
			a.State = "failed"
		}
	})
	s.emit("subagent", map[string]any{"id": id, "kind": kind, "title": title, "state": "start"})
	msgs := []Message{{Role: "user", Content: []Block{{Type: "text", Text: task}}, TS: time.Now().UnixMilli()}}
	var breaker loopBreaker
	for iter := 0; iter < 60; iter++ {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		res, done, out, err := s.subagentTurn(ctx, provider, specs, kind, run, id, iter, &msgs)
		if err != nil {
			return "", err
		}
		if done {
			return out, nil
		}
		results, err := s.subagentToolTurn(ctx, rules, allowed, res.Content, run, id, &breaker)
		if err != nil {
			return "", err
		}
		msgs = append(msgs, Message{Role: "user", Content: results})
	}
	return "", errMsg(M("agent.step_limit"))
}

func (s *Session) subagentSetup(ctx context.Context, kind string) (Provider, *RuleSet, []ToolSpec, func(string) bool, error) {
	provider, err := providerFor(s.settings, s.providerID())
	if err != nil {
		return nil, nil, nil, nil, err
	}
	rules := LoadRules(s.Root)
	mcp.For(s.Root).Ensure(ctx)
	allowed := func(name string) bool {
		if subagentAllows(kind, name) {
			return true
		}
		t, ok := s.lookupTool(name) // read-only connector tools serve the read-only kinds too
		return ok && strings.HasPrefix(name, "mcp__") && (kind == "general" || t.Category() == "read")
	}
	var specs []ToolSpec
	for _, sp := range s.toolSpecs() {
		if allowed(sp.Name) {
			specs = append(specs, sp)
		}
	}
	return provider, rules, specs, allowed, nil
}

// subagentTurn asks the model once and records its cost and trail.
func (s *Session) subagentTurn(ctx context.Context, provider Provider, specs []ToolSpec, kind string, run int, id string, iter int, msgs *[]Message) (ModelResult, bool, string, error) {
	res, err := provider.Stream(ctx, ModelRequest{Model: s.Model, System: subagentSystem(s, kind), Messages: *msgs, Tools: specs, MaxTokens: s.settings.MaxTokens}, func(StreamEvent) {})
	if err != nil {
		return res, false, "", err
	}
	cost := s.settings.CostOf(s.Model, res.Usage)
	s.Mu.Lock()
	s.Cost += cost
	s.Usage.Input += res.Usage.Input
	s.Usage.Output += res.Usage.Output
	s.Mu.Unlock()
	recordUsage(usageEntry{Root: s.Root, Session: s.ID, Provider: s.providerID(), Model: s.Model, Input: res.Usage.Input, Output: res.Usage.Output, Cost: cost})
	*msgs = append(*msgs, Message{Role: "assistant", Content: res.Content})
	for _, b := range res.Content { // what the subagent says between its steps joins its trail
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" && res.StopReason == "tool_use" {
			text := truncate(strings.TrimSpace(b.Text), 1200)
			s.agentUpdate(run, func(a *AgentRun) { a.addStep(AgentStep{Kind: "text", Text: text}) })
		}
	}
	if res.StopReason != "tool_use" {
		out := strings.TrimSpace(msgText(Message{Content: res.Content}))
		s.emit("subagent", map[string]any{"id": id, "state": "done", "steps": iter})
		s.agentUpdate(run, func(a *AgentRun) { a.State, a.Result = "done", truncate(out, 12000) }) // steps stay the tool calls counted live
		return res, true, out, nil
	}
	return res, false, "", nil
}

func (s *Session) subagentToolTurn(ctx context.Context, rules *RuleSet, allowed func(string) bool, content []Block, run int, id string, breaker *loopBreaker) ([]Block, error) {
	var results []Block
	for _, b := range content {
		if b.Type != "tool_use" {
			continue
		}
		out, isErr, err := s.subagentTool(ctx, rules, allowed, b, run, id, breaker)
		if err != nil {
			return nil, err
		}
		results = append(results, Block{Type: "tool_result", ToolUseID: b.ID, Content: out, IsError: isErr})
	}
	return results, nil
}

func (s *Session) subagentTool(ctx context.Context, rules *RuleSet, allowed func(string) bool, b Block, run int, id string, breaker *loopBreaker) (string, bool, error) {
	input := truncate(string(b.Input), 2000)
	step := -1
	s.agentUpdate(run, func(a *AgentRun) { // the step shows while it runs, as the main agent's do
		a.Steps++
		a.Tool, a.Summary = b.Name, truncate(firstLine(string(b.Input)), 160)
		step = a.addStep(AgentStep{Kind: "tool", Tool: b.Name, Input: input, State: "running", Started: time.Now().UnixMilli()})
	})
	t0 := time.Now()
	var out string
	var isErr bool
	if !allowed(b.Name) {
		out, isErr = s.say(b.ID, M("agent.tool_unavailable", "tool", b.Name)), true
	} else {
		out, isErr = s.safeRunTool(ctx, rules, b)
		out = redactSecrets(out)
	}
	display := displayFor(s.uiLang(), out, s.takeCallMsgs(b.ID)) // the trail shows infuse's messages in the user's language
	s.emit("subagent", map[string]any{"id": id, "state": "tool", "tool": b.Name, "summary": firstLine(string(b.Input))})
	ms := time.Since(t0).Milliseconds()
	s.agentUpdate(run, func(a *AgentRun) {
		if step >= 0 && step < len(a.Trail) {
			a.Trail[step].State, a.Trail[step].Ms, a.Trail[step].Output = map[bool]string{true: "error", false: "ok"}[isErr], ms, truncate(out, 1500)
			if display != "" {
				a.Trail[step].Display = truncate(display, 1500)
			}
		}
	})
	if err := breaker.record(b.Name, b.Input, isErr); err != nil {
		return "", false, errMsg(M("agent.stopped", "error", errArg(err)))
	}
	return out, isErr, nil
}

// reviewMaterial is what the reviewer sees: the request, the plan, the criteria and the diff.
func (s *Session) reviewMaterial(focus string) string {
	var b strings.Builder
	s.reviewPreamble(&b, focus)
	diff := s.reviewChanges()
	if len(diff) > 60000 {
		diff = diff[:60000] + "\n[… diff cut]"
	}
	if strings.TrimSpace(diff) == "" {
		diff = "(no changes found)"
	}
	b.WriteString("## Changes\n```diff\n" + diff + "\n```\n")
	return b.String()
}

func (s *Session) reviewPreamble(b *strings.Builder, focus string) {
	if req := s.turnRequest(); req != "" {
		b.WriteString("## User's request\n" + req + "\n\n")
	}
	s.Mu.Lock()
	tasks := append([]TaskItem{}, s.Tasks...)
	s.Mu.Unlock()
	if len(tasks) > 0 {
		b.WriteString("## Plan\n")
		for _, t := range tasks {
			fmt.Fprintf(b, "- [%s] %s", t.Status, t.Text)
			if t.Proof != "" {
				fmt.Fprintf(b, " (proof: %s)", t.Proof)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if o := readOracle(s.Root); o != "" {
		b.WriteString("## Definition of done\n" + o + "\n\n")
	}
	if g := s.groundsBrief(); g != "" {
		b.WriteString("## Project grounds\n" + g + "\n")
	}
	if m := memoryPrompt(s); m != "" {
		b.WriteString("## Memory (corrections and agreed criteria)\n" + m + "\n")
	}
	if focus != "" {
		b.WriteString("## Requested focus\n" + focus + "\n\n")
	}
}

func (s *Session) reviewChanges() string {
	root := s.workRoot()
	const budget = 60000
	if exists(filepath.Join(root, ".git")) {
		return reviewGitDiff(root, budget)
	}
	return s.reviewCheckpointFiles(root, budget)
}

func reviewGitDiff(root string, budget int) string {
	diff, _ := gitRun(root, "diff", "HEAD")
	if untracked, _ := gitRun(root, "ls-files", "--others", "--exclude-standard"); strings.TrimSpace(untracked) != "" {
		for _, f := range strings.Split(strings.TrimSpace(untracked), "\n") {
			if len(diff) > budget {
				break
			}
			if c, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil && len(c) < 40000 {
				diff += "\n--- /dev/null\n+++ b/" + f + " (new file)\n" + string(c)
			}
		}
	}
	return diff
}

func (s *Session) reviewCheckpointFiles(root string, budget int) string {
	s.Mu.Lock()
	seen := map[string]bool{}
	var files []string
	for _, cps := range s.Checkpoints {
		for _, c := range cps {
			if !seen[c.Path] {
				seen[c.Path] = true
				files = append(files, c.Path)
			}
		}
	}
	s.Mu.Unlock()
	diff := ""
	for _, f := range files {
		if c, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f))); err == nil && len(diff) < budget {
			diff += "\n=== " + f + " (changed in this session)\n" + string(c)
		}
	}
	return diff
}

// ---------------------------------------------------------------- the agent's tool

type agentTool struct{}

func (agentTool) Category() string { return "ui" }
func (agentTool) Spec() ToolSpec {
	return ToolSpec{Name: "Agent", Description: "Delegates a task to a subagent with a clean context, which returns only its conclusion. Several explore/review subagents called in the same reply work at the same time (a team): split independent research among them. kind=explore: research and reading in the project (read-only). kind=review: independent reviewer of the current changes (sees the diff, the plan and the criteria, not your reasoning) — use it before declaring a piece of work done. kind=general: a bounded task with any tool. The subagent's actions go through the same permissions and the filter.",
		InputSchema: schema(`{"type":"object","properties":{"kind":{"type":"string","enum":["explore","review","general"]},"description":{"type":"string","description":"short title"},"prompt":{"type":"string","description":"the full task: the subagent does not see this conversation"}},"required":["kind","prompt"]}`)}
}

func (agentTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Kind        string `json:"kind"`
		Description string `json:"description"`
		Prompt      string `json:"prompt"`
	}
	_ = json.Unmarshal(input, &in)
	task := in.Prompt
	if in.Kind == "review" {
		task = tc.S.reviewMaterial(in.Prompt) + "\nReview the changes above."
	}
	out, err := tc.S.RunSubagent(ctx, in.Kind, in.Description, task)
	if err != nil {
		return tc.Say(M("agent.failed", "error", errArg(err))), true
	}
	return out, false
}

// IndependentReview runs the reviewer on the current changes (asked by the user) and records its
// findings in the conversation, where the agent sees them on the next turn. The two messages say what
// they are by their Kind (review, review_ack): the screen never reads their text to find out.
func (s *Session) IndependentReview(ctx context.Context, focus string) (string, error) {
	s.Mu.Lock()
	busy := s.State != "idle" && s.State != "error"
	s.Mu.Unlock()
	if busy {
		return "", userError(s.uiLang(), M("review.busy"))
	}
	out, err := s.RunSubagent(ctx, "review", M("review.title").Render(s.uiLang()), s.reviewMaterial(focus)+"\nReview the changes above.")
	if err != nil {
		if m, ok := msgOf(err); ok {
			return "", userError(s.uiLang(), m)
		}
		return "", err
	}
	now := time.Now().UnixMilli()
	s.Mu.Lock()
	// the heading and the findings are separate blocks: the model reads both, the screen shows the findings
	review := Message{Role: "user", Kind: "review", Content: []Block{s.textBlockLocked(M("review.header")), {Type: "text", Text: out}}, TS: now}
	ack := Message{Role: "assistant", Kind: "review_ack", Content: []Block{s.textBlockLocked(M("review.ack"))}, TS: now}
	s.Messages = append(s.Messages, review, ack)
	s.Mu.Unlock()
	s.save()
	s.emit("session", s.snapshot())
	return out, nil
}

// AgentStep is one step of a subagent: what it said, or a tool it ran (input, state, time, output).
type AgentStep struct {
	Kind    string `json:"kind"` // text | tool
	Text    string `json:"text,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Input   string `json:"input,omitempty"`
	State   string `json:"state,omitempty"` // running | ok | error
	Ms      int64  `json:"ms,omitempty"`
	Output  string `json:"output,omitempty"`
	Display string `json:"display,omitempty"` // the output with infuse's messages in the user's language
	Started int64  `json:"started,omitempty"`
}

// addStep appends to the trail (the last 150 steps are kept) and returns the step's index.
func (a *AgentRun) addStep(st AgentStep) int {
	a.Trail = append(a.Trail, st)
	if len(a.Trail) > 150 {
		a.Trail = a.Trail[len(a.Trail)-150:]
	}
	return len(a.Trail) - 1
}

// AgentRun is one subagent's work, kept with the session and shown live in the Agents pane.
type AgentRun struct {
	ID      string      `json:"id"`
	Kind    string      `json:"kind"`
	Title   string      `json:"title"`
	Task    string      `json:"task"`
	State   string      `json:"state"` // running | done | failed
	Steps   int         `json:"steps"`
	Tool    string      `json:"tool,omitempty"`
	Summary string      `json:"summary,omitempty"`
	Result  string      `json:"result,omitempty"`
	Started int64       `json:"started"`
	Ended   int64       `json:"ended,omitempty"`
	Trail   []AgentStep `json:"trail,omitempty"`
}

func (s *Session) agentStart(id, kind, title, task string) int {
	s.Mu.Lock()
	s.Agents = append(s.Agents, AgentRun{ID: id, Kind: kind, Title: firstNonEmpty(title, truncate(firstLine(task), 80)), Task: truncate(task, 4000), State: "running", Started: time.Now().UnixMilli()})
	if len(s.Agents) > 200 {
		s.Agents = s.Agents[len(s.Agents)-200:]
	}
	i := len(s.Agents) - 1
	list := append([]AgentRun{}, s.Agents...)
	s.Mu.Unlock()
	s.emit("agents", list)
	return i
}

// agentUpdate changes one run (found by position, then by id if the list moved) and tells the page.
func (s *Session) agentUpdate(i int, f func(*AgentRun)) {
	s.Mu.Lock()
	if i >= len(s.Agents) {
		i = len(s.Agents) - 1
	}
	if i >= 0 {
		f(&s.Agents[i])
		if s.Agents[i].State != "running" && s.Agents[i].Ended == 0 {
			s.Agents[i].Ended = time.Now().UnixMilli()
		}
	}
	list := append([]AgentRun{}, s.Agents...)
	s.Mu.Unlock()
	s.emit("agents", list)
}
