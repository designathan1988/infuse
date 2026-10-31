package main

// Terminal pane: interactive shells in the session's folder. Windows uses a pseudo console (ConPTY,
// terminal_windows.go); macOS/Linux run the shell under `script` for a pty without extra libraries.
// Output streams to the page over SSE; keystrokes and resizes arrive as POSTs.

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

type ptyProcess interface {
	io.ReadWriter
	Resize(cols, rows int) error
	Close() error
}

type termSession struct {
	ID   string
	Root string
	Cwd  string
	pty  ptyProcess

	mu      sync.Mutex
	backlog []byte
	subs    map[chan []byte]struct{}
	closed  bool
}

var (
	terms   = map[string]*termSession{}
	termsMu sync.Mutex
)

func OpenTerminal(root, cwd string, cols, rows int) (*termSession, error) {
	if cwd == "" {
		cwd = root
	}
	if cols <= 0 {
		cols = 100
	}
	if rows <= 0 {
		rows = 30
	}
	p, err := startPTY(cwd, cols, rows)
	if err != nil {
		return nil, err
	}
	t := &termSession{ID: "t" + randomHex(6), Root: root, Cwd: cwd, pty: p, subs: map[chan []byte]struct{}{}}
	termsMu.Lock()
	terms[t.ID] = t
	termsMu.Unlock()
	go t.pump()
	return t, nil
}

func (t *termSession) pump() {
	buf := make([]byte, 32*1024)
	for {
		n, err := t.pty.Read(buf)
		if n > 0 {
			chunk := append([]byte{}, buf[:n]...)
			t.mu.Lock()
			t.backlog = append(t.backlog, chunk...)
			if len(t.backlog) > 256*1024 {
				t.backlog = t.backlog[len(t.backlog)-256*1024:]
			}
			for ch := range t.subs {
				select {
				case ch <- chunk:
				default:
				}
			}
			t.mu.Unlock()
		}
		if err != nil {
			break
		}
	}
	t.mu.Lock()
	t.closed = true
	for ch := range t.subs {
		close(ch)
	}
	t.subs = map[chan []byte]struct{}{}
	t.mu.Unlock()
}

func getTerminal(id string) *termSession {
	termsMu.Lock()
	defer termsMu.Unlock()
	return terms[id]
}

func CloseTerminal(id string) {
	termsMu.Lock()
	t := terms[id]
	delete(terms, id)
	termsMu.Unlock()
	if t != nil {
		_ = t.pty.Close()
	}
}

// streamTerminal sends the backlog, then live output, base64-encoded, as SSE.
func streamTerminal(w http.ResponseWriter, r *http.Request, t *termSession) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming não suportado", 500)
		return
	}
	w.Header().Set("content-type", "text/event-stream")
	w.Header().Set("cache-control", "no-cache")
	ch := make(chan []byte, 512)
	t.mu.Lock()
	backlog := append([]byte{}, t.backlog...)
	closed := t.closed
	if !closed {
		t.subs[ch] = struct{}{}
	}
	t.mu.Unlock()
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
	defer func() { t.mu.Lock(); delete(t.subs, ch); t.mu.Unlock() }()
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
