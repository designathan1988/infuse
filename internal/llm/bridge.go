package llm

// The desktop bridges. Claude Code and Codex are installed on this machine with their own
// authentication (a claude.ai sign-in, a ChatGPT sign-in); infuse borrows that model access without
// borrowing their agents: each turn, infuse sends the conversation and its own tool catalog to the CLI
// and the CLI answers with one structured reply. infuse then executes every tool itself, through its own
// gates, journal and checkpoints — the coding happens inside infuse, the CLI is only the pipe to the
// model. Nothing the CLI could run reaches the project: Claude Code runs with its whole tool set
// disabled (`--tools ""`), Codex under a read-only sandbox in a scratch directory.
//
// Where the CLIs are found: bridge_cli.go. How a turn is written and the reply read: bridge_proto.go.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

func init() {
	RegisterProvider("claudedesk", func(cfg ProviderConfig) Provider {
		return &bridgeProvider{kind: "claude", cli: orDefault(cfg.BaseURL, FindClaudeCLI())}
	})
	RegisterProvider("codexdesk", func(cfg ProviderConfig) Provider {
		return &bridgeProvider{kind: "codex", cli: orDefault(cfg.BaseURL, FindCodexCLI())}
	})
}

// ---------------------------------------------------------------- the provider

type bridgeProvider struct {
	kind string // "claude" | "codex"
	cli  string // executable path ("" → not installed)
}

// claudeNotLoggedIn: the CLI answered with its sign-in error instead of a model answer.
func claudeNotLoggedIn(text string) bool {
	l := strings.ToLower(text)
	return strings.Contains(l, "not logged in") || strings.Contains(l, "please run /login") || strings.Contains(l, "invalid api key")
}

func (p *bridgeProvider) Name() string {
	if p.kind == "codex" {
		return "codexdesk"
	}
	return "claudedesk"
}

// runWithDesktopFallback runs the turn and, when Claude's standalone CLI answers with its sign-in
// error, retries once with the CLI bundled with the desktop application.
func (p *bridgeProvider) runWithDesktopFallback(ctx context.Context, req ModelRequest, prompt string, emit func(StreamEvent), usage *Usage, think *strings.Builder) (string, error) {
	final, err := p.runOnce(ctx, req, prompt, emit, usage, think)
	if err != nil || p.kind != "claude" || !claudeNotLoggedIn(final) {
		return final, err
	}
	desk := FindClaudeDesktopCLI()
	if desk == "" || desk == p.cli {
		return final, nil
	}
	p.cli = desk
	*usage = Usage{}
	think.Reset()
	return p.runOnce(ctx, req, prompt, emit, usage, think)
}

// readReply parses the model's answer; when it does not follow the protocol, one repair round goes
// back to the model, and plain prose is accepted as the answer after that.
func (p *bridgeProvider) readReply(ctx context.Context, req ModelRequest, prompt, final string, emit func(StreamEvent), usage *Usage, think *strings.Builder) (bridgeReply, error) {
	reply, perr := parseBridgeReply(final)
	if perr == nil {
		return reply, nil
	}
	fix := fmt.Sprintf("\n\n[user] Your previous reply was not valid for this protocol (%v). Reply again with ONLY the JSON object, no fences, no commentary.", perr)
	final, err := p.runOnce(ctx, req, prompt+fix, emit, usage, think)
	if err != nil {
		return bridgeReply{}, err
	}
	reply, perr = bridgeReplyOrText(final)
	if perr != nil {
		return bridgeReply{}, fmt.Errorf("the bridge could not read the model's response: %w", perr)
	}
	return reply, nil
}

func (p *bridgeProvider) Stream(ctx context.Context, req ModelRequest, emit func(StreamEvent)) (ModelResult, error) {
	if p.cli == "" {
		return ModelResult{}, fmt.Errorf("%s was not found on this machine (install the application or the CLI)", p.missingName())
	}
	prompt := bridgePrompt(req)
	usage := Usage{}
	var think strings.Builder
	final, err := p.runWithDesktopFallback(ctx, req, prompt, emit, &usage, &think)
	if err != nil {
		return ModelResult{Usage: usage}, err
	}
	reply, err := p.readReply(ctx, req, prompt, final, emit, &usage, &think)
	if err != nil {
		return ModelResult{Usage: usage}, err
	}
	res := ModelResult{Usage: usage}
	if t := strings.TrimSpace(think.String()); t != "" {
		res.Content = append(res.Content, Block{Type: "thinking", Thinking: t})
	}
	if reply.Text != "" {
		for _, ch := range chunkText(reply.Text, 24) {
			emit(StreamEvent{Kind: "text", Text: ch})
		}
		res.Content = append(res.Content, Block{Type: "text", Text: reply.Text})
	}
	for i, call := range reply.ToolCalls {
		if call.Name == "" {
			continue
		}
		id := call.ID
		if id == "" {
			id = fmt.Sprintf("bt_%d_%d", time.Now().UnixMilli(), i+1)
		}
		in := call.Arguments
		if len(in) == 0 || strings.TrimSpace(string(in)) == "" {
			in = json.RawMessage("{}")
		}
		blk := Block{Type: "tool_use", ID: id, Name: call.Name, Input: in}
		emit(StreamEvent{Kind: "tool_start", ID: id, Name: call.Name, Index: i})
		emit(StreamEvent{Kind: "tool_input", ID: id, Text: string(in), Index: i})
		emit(StreamEvent{Kind: "tool_end", ID: id, Name: call.Name, Index: i})
		res.Content = append(res.Content, blk)
	}
	if len(reply.ToolCalls) > 0 {
		res.StopReason = "tool_use"
	} else {
		res.StopReason = "end_turn"
	}
	if len(res.Content) == 0 {
		res.Content = append(res.Content, Block{Type: "text", Text: core.Truncate(final, 4000)})
	}
	return res, nil
}

func (p *bridgeProvider) missingName() string {
	if p.kind == "codex" {
		return "Codex"
	}
	return "Claude Code"
}

// runOnce starts one CLI turn and returns the model's final text.
func (p *bridgeProvider) runOnce(ctx context.Context, req ModelRequest, prompt string, emit func(StreamEvent), usage *Usage, think *strings.Builder) (string, error) {
	scratch, _ := os.MkdirTemp("", "infuse-bridge-*")
	defer func() { _ = os.RemoveAll(scratch) }()
	cmd := p.bridgeCommand(req)
	cmd.Dir = scratch
	cmd.Env = append(os.Environ(), "CI=1")
	cmd.Stdin = strings.NewReader(prompt)
	process.NoConsoleWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("could not start %s (%s): %w", p.missingName(), p.cli, err)
	}
	done := make(chan struct{})
	defer close(done) // releases the killer goroutine when the turn ends on its own
	var killOnce sync.Once
	go func() { // the loop cancels the turn (Esc): the CLI dies with it
		select {
		case <-ctx.Done():
			killOnce.Do(func() { _ = cmd.Process.Kill() })
		case <-done:
		}
	}()
	finalText, err := p.readEvents(ctx, cmd, stdout, emit, usage, think)
	if err != nil {
		return finalText, err
	}
	if ctx.Err() != nil && finalText == "" {
		return "", ctx.Err()
	}
	if finalText == "" {
		return "", fmt.Errorf("%s ended without an answer (check the app's login)", p.missingName())
	}
	return finalText, nil
}

// bridgeCommand assembles the CLI invocation: claude in one-shot pipe mode with every tool off,
// codex exec under a read-only sandbox; both read the turn from stdin.
func (p *bridgeProvider) bridgeCommand(req ModelRequest) *exec.Cmd {
	if p.kind == "claude" {
		args := []string{"-p", "-", "--output-format", "stream-json", "--verbose", "--include-partial-messages",
			"--tools", "", "--strict-mcp-config", "--setting-sources", "", "--model", req.Model}
		if strings.TrimSpace(req.System) != "" {
			args = append(args, "--system-prompt", req.System)
		}
		return exec.Command(p.cli, args...)
	}
	args := []string{"exec", "--json", "--sandbox", "read-only", "--skip-git-repo-check"}
	if strings.TrimSpace(req.Model) != "" {
		args = append(args, "-m", req.Model)
	}
	args = append(args, "-")
	return exec.Command(p.cli, args...)
}

// readEvents consumes the CLI's JSON-lines stream until the turn ends, folding usage, thinking and the
// final text into the turn's state.
func (p *bridgeProvider) readEvents(ctx context.Context, cmd *exec.Cmd, stdout io.Reader, emit func(StreamEvent), usage *Usage, think *strings.Builder) (string, error) {
	finalText := ""
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	for sc.Scan() {
		if ctx.Err() != nil {
			break
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev map[string]any
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if err := p.applyEvent(ev, emit, usage, think, &finalText); err != nil {
			_ = cmd.Wait()
			return finalText, err
		}
	}
	_ = cmd.Wait()
	return finalText, nil
}

// applyEvent folds one stream event into the turn's state; only a failed turn returns an error.
func (p *bridgeProvider) applyEvent(ev map[string]any, emit func(StreamEvent), usage *Usage, think *strings.Builder, finalText *string) error {
	switch ev["type"] {
	case "stream_event": // claude, --include-partial-messages
		claudeDelta(ev, emit, think)
	case "assistant": // claude: the whole message
		claudeMessage(ev["message"], usage, finalText)
	case "result": // claude: totals of the turn
		if u := msgUsage(ev["usage"]); u.Input+u.Output > 0 {
			usage.Input, usage.Output = u.Input, u.Output
		}
	case "item.completed": // codex
		codexItem(ev["item"], think, finalText)
	case "turn.completed": // codex: totals
		u := msgUsage(ev["usage"])
		usage.Input += u.Input
		usage.Output += u.Output
	case "turn.failed", "error":
		if msg := firstString(ev["error"], ev["message"]); msg != "" {
			return fmt.Errorf("%s: %s", p.missingName(), core.Truncate(msg, 400))
		}
	}
	return nil
}

// claudeDelta emits the partial thinking of one claude stream event.
func claudeDelta(ev map[string]any, emit func(StreamEvent), think *strings.Builder) {
	event, _ := ev["event"].(map[string]any)
	delta, _ := event["delta"].(map[string]any)
	if delta == nil {
		return
	}
	if t, _ := delta["thinking"].(string); t != "" {
		think.WriteString(t)
		emit(StreamEvent{Kind: "thinking", Text: t})
	}
}

// claudeMessage keeps the last text of the assistant message and adds its usage.
func claudeMessage(v any, usage *Usage, finalText *string) {
	msg, _ := v.(map[string]any)
	for _, c := range asBlocks(msg["content"]) {
		if c["type"] == "text" {
			if t, _ := c["text"].(string); t != "" {
				*finalText = t
			}
		}
	}
	u := msgUsage(msg["usage"])
	usage.Input += u.Input
	usage.Output += u.Output
	usage.CacheRead += u.CacheRead
	usage.CacheWrite += u.CacheWrite
}

// codexItem keeps the last agent message and the reasoning of one codex item.
func codexItem(v any, think *strings.Builder, finalText *string) {
	item, _ := v.(map[string]any)
	switch item["type"] {
	case "agent_message":
		if t, _ := item["text"].(string); t != "" {
			*finalText = t
		}
	case "reasoning":
		if t, _ := item["text"].(string); t != "" {
			think.WriteString(t + "\n")
		}
	}
}

func asBlocks(v any) []map[string]any {
	out := []map[string]any{}
	if list, ok := v.([]any); ok {
		for _, it := range list {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func msgUsage(v any) Usage {
	m, _ := v.(map[string]any)
	num := func(k string) int {
		f, _ := m[k].(float64)
		return int(f)
	}
	return Usage{Input: num("input_tokens"), Output: num("output_tokens"),
		CacheRead: num("cache_read_input_tokens") + num("cached_input_tokens"), CacheWrite: num("cache_creation_input_tokens")}
}

func firstString(vs ...any) string {
	for _, v := range vs {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}
