package core

// CapWriter keeps the first and last `limit` bytes of a stream, so a command that prints gigabytes
// cannot exhaust memory; what was dropped is reported in the middle.
type CapWriter struct {
	limit   int
	head    []byte
	tail    []byte
	dropped int64
}

func NewCapWriter(limit int) *CapWriter { return &CapWriter{limit: limit} }

func (w *CapWriter) Write(p []byte) (int, error) {
	n := len(p)
	if room := w.limit - len(w.head); room > 0 {
		k := min(room, len(p))
		w.head = append(w.head, p[:k]...)
		p = p[k:]
	}
	if len(p) > 0 {
		w.tail = append(w.tail, p...)
		if over := len(w.tail) - w.limit; over > 0 {
			w.dropped += int64(over)
			w.tail = append(w.tail[:0], w.tail[over:]...)
		}
	}
	return n, nil
}

func (w *CapWriter) String() string {
	if w.dropped == 0 {
		return string(w.head) + string(w.tail)
	}
	return string(w.head) + "\n\n" + M("safety.omitted", "bytes", w.dropped).String() + "\n\n" + string(w.tail)
}
