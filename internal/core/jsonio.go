package core

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
)

func Exists(p string) bool { _, err := os.Stat(p); return err == nil }

// RawJSON is a literal JSON document (tool input schemas).
func RawJSON(s string) json.RawMessage { return json.RawMessage(s) }

func ReadJSON(file string, v any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// WriteJSON writes a document atomically: the new content goes to a temporary file in the same
// directory, is flushed, and replaces the old file in one rename. A crash never leaves a half-written
// document.
func WriteJSON(file string, v any) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(file), ".infuse-tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, file); err != nil {
		cleanup()
		return err
	}
	return nil
}

// AppendLine appends one JSON document per line (the journal and usage ledgers).
func AppendLine(file string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// ReadJSONL reads a JSON-lines file, skipping malformed lines.
func ReadJSONL(file string) []map[string]any {
	f, err := os.Open(file)
	if err != nil {
		return []map[string]any{}
	}
	defer f.Close()
	out := []map[string]any{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var m map[string]any
		if json.Unmarshal(sc.Bytes(), &m) == nil {
			out = append(out, m)
		}
	}
	return out
}
