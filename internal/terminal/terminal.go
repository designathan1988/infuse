// Package terminal hosts interactive shells in a project folder: ConPTY on Windows, `script` on
// macOS/Linux. It owns the processes and the output fan-out; the HTTP/SSE adapter lives in the API.
package terminal

import (
	"infuse/internal/core"
	"io"
	"sync"
)

type PTY interface {
	io.ReadWriter
	Resize(cols, rows int) error
	Close() error
}

type Session struct {
	ID   string
	Root string
	Cwd  string
	pty  PTY

	mu      sync.Mutex
	backlog []byte
	subs    map[chan []byte]struct{}
	closed  bool
}

var (
	terms   = map[string]*Session{}
	termsMu sync.Mutex
)

func Open(root, cwd string, cols, rows int) (*Session, error) {
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
	t := &Session{ID: "t" + core.RandomHex(6), Root: root, Cwd: cwd, pty: p, subs: map[chan []byte]struct{}{}}
	termsMu.Lock()
	terms[t.ID] = t
	termsMu.Unlock()
	go t.pump()
	return t, nil
}

func (t *Session) pump() {
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

func Get(id string) *Session {
	termsMu.Lock()
	defer termsMu.Unlock()
	return terms[id]
}

func Close(id string) {
	termsMu.Lock()
	t := terms[id]
	delete(terms, id)
	termsMu.Unlock()
	if t != nil {
		_ = t.pty.Close()
	}
}

func (t *Session) Write(p []byte) (int, error) { return t.pty.Write(p) }

func (t *Session) Resize(cols, rows int) error { return t.pty.Resize(cols, rows) }

// Output returns the output kept so far (a bounded backlog).
func (t *Session) Output() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.backlog)
}

// Attach returns the output so far, a channel for the live output, and whether the shell already
// ended. The caller must Detach the channel when it stops reading.
func (t *Session) Attach() (backlog []byte, ch chan []byte, closed bool) {
	ch = make(chan []byte, 512)
	t.mu.Lock()
	backlog = append([]byte{}, t.backlog...)
	closed = t.closed
	if !closed {
		t.subs[ch] = struct{}{}
	}
	t.mu.Unlock()
	return backlog, ch, closed
}

func (t *Session) Detach(ch chan []byte) {
	t.mu.Lock()
	delete(t.subs, ch)
	t.mu.Unlock()
}

// CloseAll ends every terminal (application shutdown).
func CloseAll() {
	termsMu.Lock()
	ids := make([]string, 0, len(terms))
	for id := range terms {
		ids = append(ids, id)
	}
	termsMu.Unlock()
	for _, id := range ids {
		Close(id)
	}
}
