package browser

// A headless Chromium-family browser (Edge, Chrome, Chromium — whichever is installed) driven over the
// DevTools protocol, for the agent's Browser tool. It never shows a window. One browser per project,
// with its own profile under ~/.infuse, started on first use and closed after 15 idle minutes.

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ---------------------------------------------------------------- minimal WebSocket client (RFC 6455)

type wsConn struct {
	c  net.Conn
	br *bufio.Reader
	mu sync.Mutex
}

func wsDial(raw string) (*wsConn, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	c, err := net.DialTimeout("tcp", u.Host, 10*time.Second)
	if err != nil {
		return nil, err
	}
	keyb := make([]byte, 16)
	_, _ = rand.Read(keyb)
	key := base64.StdEncoding.EncodeToString(keyb)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", u.RequestURI(), u.Host, key)
	if _, err := c.Write([]byte(req)); err != nil {
		c.Close()
		return nil, err
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		c.Close()
		return nil, err
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if resp.StatusCode != 101 || resp.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(sum[:]) {
		c.Close()
		return nil, fmt.Errorf("websocket: resposta %d", resp.StatusCode)
	}
	return &wsConn{c: c, br: br}, nil
}

func (w *wsConn) writeText(p []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	hdr := []byte{0x81}
	n := len(p)
	switch {
	case n < 126:
		hdr = append(hdr, byte(n)|0x80)
	case n < 65536:
		hdr = append(hdr, 126|0x80, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127|0x80)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(hdr, b[:]...)
	}
	mask := make([]byte, 4)
	_, _ = rand.Read(mask)
	hdr = append(hdr, mask...)
	out := make([]byte, n)
	for i := range p {
		out[i] = p[i] ^ mask[i%4]
	}
	_, err := w.c.Write(append(hdr, out...))
	return err
}

// readMessage returns the next complete text or binary message (control frames handled here).
func (w *wsConn) readMessage() ([]byte, error) {
	var msg []byte
	for {
		h := make([]byte, 2)
		if _, err := io.ReadFull(w.br, h); err != nil {
			return nil, err
		}
		fin, op := h[0]&0x80 != 0, h[0]&0x0f
		n := int64(h[1] & 0x7f)
		switch n {
		case 126:
			b := make([]byte, 2)
			if _, err := io.ReadFull(w.br, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint16(b))
		case 127:
			b := make([]byte, 8)
			if _, err := io.ReadFull(w.br, b); err != nil {
				return nil, err
			}
			n = int64(binary.BigEndian.Uint64(b))
		}
		if h[1]&0x80 != 0 { // servers do not mask; skip a mask if present
			if _, err := io.CopyN(io.Discard, w.br, 4); err != nil {
				return nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}
		switch op {
		case 0x8:
			return nil, io.EOF
		case 0x9: // ping → pong
			w.mu.Lock()
			_, _ = w.c.Write([]byte{0x8A, 0x80, 0, 0, 0, 0})
			w.mu.Unlock()
			continue
		case 0xA:
			continue
		}
		msg = append(msg, payload...)
		if fin {
			return msg, nil
		}
	}
}

// ---------------------------------------------------------------- CDP session
