package llm

// The bridge protocol: how one infuse turn is written for a CLI that only reads text, and how the
// model's structured reply is read back. The model never sees infuse's wire types — the transcript and
// the tool catalog are plain text, and the answer is one JSON object.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"strings"
	"unicode"
)

type bridgeReply struct {
	Text      string `json:"text"`
	ToolCalls []struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"tool_calls"`
}

// bridgePrompt serializes one infuse turn into the protocol both CLIs answer: the tool catalog, the
// transcript (assistant tool calls and their results included) and the reply contract. The system prompt
// travels as claude's --system-prompt or, for codex, as the head of the message.
func bridgePrompt(req ModelRequest) string {
	var b strings.Builder
	if req.System != "" {
		b.WriteString("SYSTEM\n")
		b.WriteString(req.System)
		b.WriteString("\n\n")
	}
	b.WriteString(`You are the model of infuse, a coding agent. infuse executes every tool for you and returns each result in the transcript; you execute and read nothing yourself. You are not Claude Code or Codex here and must not try to run commands, edit files or browse: answer only through the protocol below.

Reply with ONE JSON object and nothing else — no markdown fence, no prose before or after:
{"text": <string or null>, "tool_calls": [{"id": "<unique>", "name": "<tool name>", "arguments": {}}]}
- To act: put the calls in "tool_calls" (as many as make sense together; each "arguments" must match the tool's schema exactly). "text" is then a short note to the user, or null.
- To finish: "text" carries the answer for the user and "tool_calls" is [].
- Never invent tools, ids or results. Never wrap the JSON in anything.

TOOLS
`)
	b.WriteString(bridgeTools(req.Tools))
	b.WriteString("\n\nTRANSCRIPT\n")
	b.WriteString(bridgeTranscript(req.Messages))
	b.WriteString("\nReply with the JSON object now.")
	if req.ForceTool != "" {
		b.WriteString(fmt.Sprintf(" The host requires exactly one call in this reply: the tool %q with the requested answer as its arguments.", req.ForceTool))
	}
	return b.String()
}

func bridgeTools(tools []ToolSpec) string {
	list := make([]string, 0, len(tools))
	for _, t := range tools {
		list = append(list, fmt.Sprintf(`- %s: %s
  arguments schema: %s`, t.Name, strings.TrimSpace(t.Description), string(t.InputSchema)))
	}
	return strings.Join(list, "\n")
}

// bridgeTranscript renders the conversation so any model can continue it: user texts, the assistant's
// texts and tool calls, and each tool's result answering its call.
func bridgeTranscript(msgs []Message) string {
	msgs = RepairTranscript(msgs)
	var b strings.Builder
	for _, m := range msgs {
		role := "user"
		if m.Role == "assistant" {
			role = "assistant"
		}
		var parts []string
		for _, blk := range m.Content {
			if s := bridgeBlock(blk); s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[%s] %s\n", role, strings.Join(parts, "\n"))
	}
	return strings.TrimSpace(b.String())
}

func bridgeBlock(blk Block) string {
	switch blk.Type {
	case "text":
		return blk.Text
	case "thinking":
		if blk.Thinking != "" {
			return "(reasoning) " + core.Truncate(blk.Thinking, 2000)
		}
	case "image":
		return fmt.Sprintf("(image %s omitted: attach it again if you still need it)", blk.MediaType)
	case "tool_use":
		return fmt.Sprintf("(tool call %s) %s %s", blk.ID, blk.Name, string(blk.Input))
	case "tool_result":
		content := blk.Content
		if content == "" {
			content = "(no output)"
		}
		if blk.IsError {
			content = "ERROR: " + content
		}
		return fmt.Sprintf("(result of %s) %s", blk.ToolUseID, content)
	}
	return ""
}

// bridgeReplyOrText is the last chance after a repair round: a model that keeps answering in plain
// prose is still answering, so the text becomes the reply instead of failing the turn. No tool call is
// ever invented from prose.
func bridgeReplyOrText(text string) (bridgeReply, error) {
	if r, err := parseBridgeReply(text); err == nil {
		return r, nil
	}
	if plain := strings.TrimSpace(text); plain != "" {
		return bridgeReply{Text: plain}, nil
	}
	return bridgeReply{}, fmt.Errorf("the reply had neither JSON nor text")
}

// parseBridgeReply finds the JSON object in the model's reply (it may sit inside a fence or carry a
// stray line before it) and checks its shape.
func parseBridgeReply(text string) (bridgeReply, error) {
	var reply bridgeReply
	raw := extractJSONObject(text)
	if raw == "" {
		return reply, fmt.Errorf("no JSON object found in the reply")
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&reply); err != nil {
		return reply, fmt.Errorf("invalid JSON: %w", err)
	}
	for _, c := range reply.ToolCalls {
		if strings.TrimSpace(c.Name) == "" {
			return reply, fmt.Errorf("tool call without a name")
		}
	}
	if reply.Text == "" && len(reply.ToolCalls) == 0 {
		return reply, fmt.Errorf("neither text nor tool_calls present")
	}
	return reply, nil
}

// extractJSONObject returns the first balanced top-level {...} in s, skipping braces inside strings.
func extractJSONObject(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
			} else if c == '\\' {
				esc = true
			} else if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

// chunkText splits text into pieces of about n words so a whole answer still streams visibly.
func chunkText(s string, n int) []string {
	words := strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) })
	if len(words) == 0 {
		return nil
	}
	var out []string
	for i := 0; i < len(words); i += n {
		end := min(i+n, len(words))
		tail := ""
		if end < len(words) {
			tail = " "
		}
		out = append(out, strings.Join(words[i:end], " ")+tail)
	}
	return out
}
