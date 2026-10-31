package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type JournalEntry struct {
	Ts      int64  `json:"ts"`
	Kind    string `json:"kind"` // write | unlink | command | transform
	Path    string `json:"path,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Command string `json:"command,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Vice    string `json:"vice,omitempty"`
}

type LessonEntry struct {
	Ts     int64  `json:"ts"`
	Vice   string `json:"vice"`
	Detail string `json:"detail"`
}

func infuseDir(root string) string {
	d := filepath.Join(root, infuseDirName)
	if err := os.MkdirAll(d, 0o755); err == nil {
		gi := filepath.Join(d, ".gitignore")
		if _, err := os.Stat(gi); err != nil {
			// Working state stays out of version control; rules, oracle, notes and memory are meant to be committed.
			_ = os.WriteFile(gi, []byte("sessions/\ncheckpoints/\nmap/\npreview/\nbin/\njournal.jsonl\nlessons.jsonl\nwatcher.json\n"), 0o644)
		}
	}
	return d
}

func appendLine(file string, v any) {
	b, _ := json.Marshal(v)
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func Journal(root string, e JournalEntry) {
	e.Ts = time.Now().UnixMilli()
	appendLine(filepath.Join(infuseDir(root), "journal.jsonl"), e)
}

func Lesson(root, vice, detail string) {
	appendLine(filepath.Join(infuseDir(root), "lessons.jsonl"), LessonEntry{time.Now().UnixMilli(), vice, detail})
}

func ReadJournalSince(root string, since int64) []JournalEntry {
	return readJournal(filepath.Join(root, infuseDirName, "journal.jsonl"), since)
}

func readJournal(file string, since int64) []JournalEntry {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []JournalEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var e JournalEntry
		if json.Unmarshal(sc.Bytes(), &e) == nil && e.Ts >= since {
			out = append(out, e)
		}
	}
	return out
}

func readJSONL(file string) []map[string]any {
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
