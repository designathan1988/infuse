// Package core holds the data types and utilities every other package may use. It depends on nothing
// inside the module.
package core

import "encoding/json"

type Block struct {
	Type      string          `json:"type"` // text | tool_use | tool_result | thinking | image
	MediaType string          `json:"mediaType,omitempty"`
	Data      string          `json:"data,omitempty"` // image: base64
	Text      string          `json:"text,omitempty"`
	Thinking  string          `json:"thinking,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
	Ms        int64           `json:"ms,omitempty"`      // tool_result: how long the tool ran; thinking: how long the model thought
	Display   string          `json:"display,omitempty"` // tool_result/text from infuse: the content in the user's language (never sent to a model)
	Meta      json.RawMessage `json:"meta,omitempty"`    // structured data infuse attached for the screen (a mission's goal, a checked report); never sent to a model
}

// Message carries, besides the content sent to providers, what the transcript shows about it: when it
// arrived and, for the agent's messages, how long the model took, which model and how many tokens.
type Message struct {
	Role    string  `json:"role"` // user | assistant
	Content []Block `json:"content"`
	TS      int64   `json:"ts,omitempty"`     // unix ms
	Ms      int64   `json:"ms,omitempty"`     // assistant: request duration
	Model   string  `json:"model,omitempty"`  // assistant
	Tokens  int     `json:"tokens,omitempty"` // assistant: output tokens
	Kind    string  `json:"kind,omitempty"`   // what the message is, for the screen, never read from its text: review | review_ack | mission | compaction | infuse_note
}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

// InlineImage is an image pasted into the prompt box.
type InlineImage struct {
	MediaType string `json:"mediaType"`
	Data      string `json:"data"`
}

type TaskItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`          // pending | in_progress | done | blocked
	Proof  string `json:"proof,omitempty"` // how the task will be shown to work (a command, a page, a file)
	Note   string `json:"note,omitempty"`  // set by infuse, never by the model (why a task was closed as blocked)
}

type CheckpointFile struct {
	Path string `json:"path"` // relative posix
	New  bool   `json:"new"`  // did not exist before the turn
}

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
	Seq  int    `json:"seq"`
}

type Attachment struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Image bool   `json:"image"`
}
