package mcp

// Streamable HTTP pieces of the JSON-RPC client.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"infuse/internal/core"
	"io"
	"net/http"
	"os"
	"strings"
)

// newPostRequest builds the POST for one message, with the configured headers and the session id.
func (c *Client) newPostRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", c.cfg.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", mcpProtocol)
	for k, v := range c.cfg.Headers {
		req.Header.Set(k, os.ExpandEnv(v))
	}
	c.mu.Lock()
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	c.mu.Unlock()
	return req, nil
}

// readEventStream reads an SSE answer, dispatching server requests and returning the response with id.
func (c *Client) readEventStream(r io.Reader, id int64) (RPCMessage, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 32<<20)
	var data strings.Builder
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			continue
		}
		if line == "" && data.Len() > 0 {
			var m RPCMessage
			if json.Unmarshal([]byte(data.String()), &m) == nil {
				if m.Method != "" {
					c.Dispatch(m)
				} else if m.ID != nil && *m.ID == id {
					return m, nil
				}
			}
			data.Reset()
		}
	}
	return RPCMessage{}, core.ErrMsg(core.M("mcp.stream_ended"))
}
