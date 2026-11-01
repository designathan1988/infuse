package journal

// The project journal and lesson ledger: every write and command that passed through the filter, and
// every vice observed. Credentials never reach these files.

import (
	"bufio"
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/data"
	"infuse/internal/security"
	"os"
	"path/filepath"
	"sync"
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
	Ts        int64  `json:"ts"`
	Vice      string `json:"vice"`
	Detail    string `json:"detail"`
	Signature string `json:"signature,omitempty"`
	Remedy    string `json:"remedy,omitempty"`
}

type viceInfo struct {
	Vice      string `json:"vice"`
	Signature string `json:"signature"`
	Remedy    string `json:"remedy"`
}

var (
	vicesOnce sync.Once
	vices     map[string]viceInfo
)

// Vice describes the observed vice by its deterministic signature and its remedy, from the base
// catalog in internal/data/vices.json. The signature is what was detected; the remedy is what to do.
func Vice(name string) (signature, remedy string) {
	vicesOnce.Do(func() {
		vices = map[string]viceInfo{}
		var list []viceInfo
		if json.Unmarshal(data.Read("vices.json"), &list) == nil {
			for _, v := range list {
				vices[v.Vice] = v
			}
		}
	})
	v, ok := vices[name]
	if !ok {
		return "", ""
	}
	return v.Signature, v.Remedy
}

// InfuseDir creates and returns the project's .infuse directory, seeding its .gitignore.
func InfuseDir(root string) string {
	d := filepath.Join(root, core.InfuseDirName)
	if err := os.MkdirAll(d, 0o755); err == nil {
		gi := filepath.Join(d, ".gitignore")
		if _, err := os.Stat(gi); err != nil {
			// Working state stays out of version control; rules, oracle, notes and memory are meant to be committed.
			_ = os.WriteFile(gi, []byte("sessions/\ncheckpoints/\nmap/\npreview/\nbin/\njournal.jsonl\nlessons.jsonl\nwatcher.json\n"), 0o644)
		}
	}
	return d
}

func Journal(root string, e JournalEntry) {
	e.Ts = time.Now().UnixMilli()
	e.Command, e.Detail = security.RedactSecrets(e.Command), security.RedactSecrets(e.Detail) // the journal never stores credentials
	core.AppendLine(filepath.Join(InfuseDir(root), "journal.jsonl"), e)
}

func Lesson(root, vice, detail string) {
	e := LessonEntry{Ts: time.Now().UnixMilli(), Vice: vice, Detail: detail}
	e.Signature, e.Remedy = Vice(vice)
	core.AppendLine(filepath.Join(InfuseDir(root), "lessons.jsonl"), e)
}

func ReadJournalSince(root string, since int64) []JournalEntry {
	return readJournal(filepath.Join(root, core.InfuseDirName, "journal.jsonl"), since)
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
