package journal

// The project journal and lesson ledger: every write and command that passed through the filter, and
// every vice observed. Credentials never reach these files.

import (
	"bytes"
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/data"
	"infuse/internal/security"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type JournalEntry struct {
	Ts      int64  `json:"ts"`
	Kind    string `json:"kind"` // write | unlink | command | transform | cost | gate | vice | ...
	Path    string `json:"path,omitempty"`
	Bytes   int    `json:"bytes,omitempty"`
	Command string `json:"command,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Vice    string `json:"vice,omitempty"`
	Millis  int64  `json:"ms,omitempty"` // how long a command took
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

// infuseIgnore is what stays out of version control: conversations, the blobs behind checkpoints and
// turn snapshots, captures, and what only makes sense on this machine. Every other artifact — the map,
// the lessons, the journal, the oracle, the evidence — is versioned (SPEC §5).
const infuseIgnore = "sessions/\ncheckpoints/\ntree/\nturnlog/\nshots/\nrecordings/\npreview/\nbin/\nwatcher.json\n"

// oldIgnores are the seeds earlier versions wrote. They are replaced; a file the user edited is not.
var oldIgnores = []string{"sessions/\ncheckpoints/\nmap/\npreview/\nbin/\njournal.jsonl\nlessons.jsonl\nwatcher.json\n"}

// infuseAttributes lets two branches append to the ledgers without a conflict.
const infuseAttributes = "journal.jsonl merge=union\nlessons.jsonl merge=union\nmap/** linguist-generated=true\n"

// InfuseDir creates and returns the project's .infuse directory, seeding what git needs to know.
func InfuseDir(root string) string {
	d := filepath.Join(root, core.InfuseDirName)
	if err := os.MkdirAll(d, 0o755); err == nil {
		seedGitFiles(d)
	}
	return d
}

func seedGitFiles(d string) {
	gi := filepath.Join(d, ".gitignore")
	b, err := os.ReadFile(gi)
	if err != nil || isOldIgnore(string(b)) {
		_ = os.WriteFile(gi, []byte(infuseIgnore), 0o644)
	}
	ga := filepath.Join(d, ".gitattributes")
	if _, err := os.Stat(ga); err != nil {
		_ = os.WriteFile(ga, []byte(infuseAttributes), 0o644)
	}
}

func isOldIgnore(text string) bool {
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	for _, o := range oldIgnores {
		if norm == o {
			return true
		}
	}
	return false
}

func Journal(root string, e JournalEntry) {
	e.Ts = time.Now().UnixMilli()
	e.Command, e.Detail = security.RedactSecrets(e.Command), security.RedactSecrets(e.Detail) // the journal never stores credentials
	core.AppendLine(filepath.Join(InfuseDir(root), "journal.jsonl"), e)
}

// Lesson records an observed vice in the ledger, with its signature and remedy, and in the journal.
func Lesson(root, vice, detail string) {
	Observe(root, JournalEntry{Kind: "vice", Detail: detail}, vice)
}

// Observe journals an event that carries a vice and adds the vice to the lesson ledger.
func Observe(root string, e JournalEntry, vice string) {
	e.Vice = vice
	Journal(root, e)
	l := LessonEntry{Ts: time.Now().UnixMilli(), Vice: vice, Detail: security.RedactSecrets(e.Detail)}
	l.Signature, l.Remedy = Vice(vice)
	core.AppendLine(filepath.Join(InfuseDir(root), "lessons.jsonl"), l)
}

func ReadJournalSince(root string, since int64) []JournalEntry {
	return readJournal(filepath.Join(root, core.InfuseDirName, "journal.jsonl"), since)
}

// journalWindow is the first read from the end of the journal: the file only grows, and the filter
// path asks for the last moments of it on every command.
const journalWindow = 256 << 10

// readJournal reads the entries from since on, walking back from the end of the file only as far as
// they go. Timestamps only grow, so the first entry older than since ends the walk.
func readJournal(file string, since int64) []JournalEntry {
	f, err := os.Open(file)
	if err != nil {
		return nil
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil
	}
	for window := int64(journalWindow); ; window *= 4 {
		start := max(st.Size()-window, 0)
		entries, reachedOlder := readJournalFrom(f, start, st.Size(), since)
		if reachedOlder || start == 0 {
			return entries
		}
	}
}

// readJournalFrom parses the entries of one window, reporting whether it reached one older than since.
func readJournalFrom(f *os.File, start, end, since int64) ([]JournalEntry, bool) {
	buf := make([]byte, end-start)
	if _, err := f.ReadAt(buf, start); err != nil && err != io.EOF {
		return nil, false
	}
	if start > 0 { // the window begins inside a line
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		}
	}
	var out []JournalEntry
	older := false
	for _, line := range bytes.Split(buf, []byte{'\n'}) {
		var e JournalEntry
		if len(line) == 0 || json.Unmarshal(line, &e) != nil {
			continue
		}
		if e.Ts >= since {
			out = append(out, e)
		} else {
			older = true
		}
	}
	return out, older
}
