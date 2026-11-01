package api

// HTTP/SSE adapter for the terminal pane. The PTY processes and their output fan-out live in
// internal/terminal.

import (
	"encoding/base64"
	"fmt"
	"infuse/internal/terminal"
	"net/http"
	"time"
)

type termSession = terminal.Session

func OpenTerminal(root, cwd string, cols, rows int) (*termSession, error) {
	return terminal.Open(root, cwd, cols, rows)
}

func getTerminal(id string) *termSession { return terminal.Get(id) }

func CloseTerminal(id string) { terminal.Close(id) }

// streamTerminal sends the backlog, then live output, base64-encoded, as SSE. The browser decodes it
// as bytes, so escape sequences reach the emulator unchanged.
func streamTerminal(w http.ResponseWriter, r *http.Request, t *termSession) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this connection", 500)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	backlog, ch, closed := t.Attach()
	send := func(b []byte) {
		fmt.Fprintf(w, "data: %s\n\n", base64.StdEncoding.EncodeToString(b))
		fl.Flush()
	}
	if len(backlog) > 0 {
		send(backlog)
	}
	if closed {
		fmt.Fprint(w, "event: exit\ndata: \n\n")
		fl.Flush()
		return
	}
	defer t.Detach(ch)
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case b, ok := <-ch:
			if !ok {
				fmt.Fprint(w, "event: exit\ndata: \n\n")
				fl.Flush()
				return
			}
			send(b)
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
