package llm

// Reading the Messages API stream (server-sent events) into a result.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type anthropicStream struct {
	res      ModelResult
	blocks   map[int]*Block
	inputBuf map[int]*strings.Builder
	order    []int
	refused  string // why the model declined, from stop_details
}

func readAnthropicStream(r io.Reader, emit func(StreamEvent)) (ModelResult, error) {
	a := &anthropicStream{blocks: map[int]*Block{}, inputBuf: map[int]*strings.Builder{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	var eventName string
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "event: ") {
			eventName = strings.TrimPrefix(line, "event: ")
			continue
		}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		if err := a.event(eventName, []byte(strings.TrimPrefix(line, "data: ")), emit); err != nil {
			return a.res, err
		}
	}
	if err := sc.Err(); err != nil {
		return a.res, err
	}
	a.content()
	switch a.res.StopReason {
	case "":
		return a.res, errors.New("the stream ended before the message did")
	case "refusal":
		return a.res, fmt.Errorf("the model declined this request (%s)", a.refused)
	}
	return a.res, nil
}

func (a *anthropicStream) event(name string, data []byte, emit func(StreamEvent)) error {
	switch name {
	case "message_start":
		var ev struct {
			Message struct {
				Usage wireUsage `json:"usage"`
			} `json:"message"`
		}
		_ = json.Unmarshal(data, &ev)
		ev.Message.Usage.into(&a.res.Usage)
	case "content_block_start":
		a.startBlock(data, emit)
	case "content_block_delta":
		a.delta(data, emit)
	case "content_block_stop":
		a.stopBlock(data, emit)
	case "message_delta":
		a.messageDelta(data)
	case "error":
		var ev struct {
			Error struct{ Type, Message string } `json:"error"`
		}
		_ = json.Unmarshal(data, &ev)
		return fmt.Errorf("stream: %s", ev.Error.Message)
	}
	return nil
}

// wireUsage is the usage object of message_start and message_delta; the second can restate the first.
type wireUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
}

func (w wireUsage) into(u *Usage) {
	for _, f := range []struct {
		to *int
		v  int
	}{{&u.Input, w.InputTokens}, {&u.Output, w.OutputTokens}, {&u.CacheRead, w.CacheReadInputTokens}, {&u.CacheWrite, w.CacheCreationInputTokens}} {
		if f.v > 0 {
			*f.to = f.v
		}
	}
}

func (a *anthropicStream) messageDelta(data []byte) {
	var ev struct {
		Delta struct {
			StopReason  string `json:"stop_reason"`
			StopDetails *struct {
				Category string `json:"category"`
			} `json:"stop_details"`
		} `json:"delta"`
		Usage wireUsage `json:"usage"`
	}
	_ = json.Unmarshal(data, &ev)
	a.res.StopReason = ev.Delta.StopReason
	if d := ev.Delta.StopDetails; d != nil {
		a.refused = d.Category
	}
	ev.Usage.into(&a.res.Usage)
}

func (a *anthropicStream) stopBlock(data []byte, emit func(StreamEvent)) {
	var ev struct {
		Index int `json:"index"`
	}
	_ = json.Unmarshal(data, &ev)
	blk := a.blocks[ev.Index]
	if blk == nil || blk.Type != "tool_use" {
		return
	}
	raw := a.inputBuf[ev.Index].String()
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	blk.Input = json.RawMessage(raw)
	emit(StreamEvent{Kind: "tool_end", ID: blk.ID, Name: blk.Name, Index: ev.Index})
}

func (a *anthropicStream) startBlock(data []byte, emit func(StreamEvent)) {
	var ev struct {
		Index        int `json:"index"`
		ContentBlock struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			Name      string `json:"name"`
			Text      string `json:"text"`
			Data      string `json:"data"` // redacted_thinking
			Thinking  string `json:"thinking"`
			Signature string `json:"signature"`
		} `json:"content_block"`
	}
	_ = json.Unmarshal(data, &ev)
	c := ev.ContentBlock
	blk := &Block{Type: c.Type, ID: c.ID, Name: c.Name, Text: c.Text, Data: c.Data, Thinking: c.Thinking, Signature: c.Signature}
	a.blocks[ev.Index] = blk
	a.order = append(a.order, ev.Index)
	if blk.Type == "tool_use" {
		a.inputBuf[ev.Index] = &strings.Builder{}
		emit(StreamEvent{Kind: "tool_start", ID: blk.ID, Name: blk.Name, Index: ev.Index})
	}
}

func (a *anthropicStream) delta(data []byte, emit func(StreamEvent)) {
	var ev struct {
		Index int `json:"index"`
		Delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			PartialJSON string `json:"partial_json"`
			Thinking    string `json:"thinking"`
			Signature   string `json:"signature"`
		} `json:"delta"`
	}
	_ = json.Unmarshal(data, &ev)
	blk := a.blocks[ev.Index]
	if blk == nil {
		return
	}
	switch ev.Delta.Type {
	case "text_delta":
		blk.Text += ev.Delta.Text
		emit(StreamEvent{Kind: "text", Text: ev.Delta.Text, Index: ev.Index})
	case "input_json_delta":
		a.inputBuf[ev.Index].WriteString(ev.Delta.PartialJSON)
		emit(StreamEvent{Kind: "tool_input", Text: ev.Delta.PartialJSON, ID: blk.ID, Index: ev.Index})
	case "thinking_delta":
		blk.Thinking += ev.Delta.Thinking
		emit(StreamEvent{Kind: "thinking", Text: ev.Delta.Thinking, Index: ev.Index})
	case "signature_delta":
		blk.Signature += ev.Delta.Signature
	}
}

// content keeps the blocks in order. A signed reasoning block stays even with no text (the default on
// current models): the API needs it back as it came.
func (a *anthropicStream) content() {
	for _, i := range a.order {
		blk := a.blocks[i]
		if blk.Type == "thinking" && blk.Thinking == "" && blk.Signature == "" {
			continue
		}
		a.res.Content = append(a.res.Content, *blk)
	}
}
