package main

// Docs before commands: a command may only use options that the agent has actually read about.
// infuse keeps the text of everything the agent read as documentation in this session (pages fetched
// in full, --help and man output, docs and package sources, skills) and, before a command runs, checks
// each long option (--name) of every specialized program against that text. An option the agent has not
// seen in what it read is refused with the list, so it reads the documentation first instead of
// guessing. A page fetched in parts must be read to the end before the agent acts: skimming is refused.
// Everyday tooling (git, go, npm, node, the shell's own utilities) and the project's own programs are
// not checked; neither are options that came from the user or from a command that already succeeded.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const (
	docCorpusMax  = 8 << 20 // text kept from what was read
	docPageSize   = 30000   // WebFetch part size
	docMustFinish = 8       // a page with up to this many parts must be read to the end
)

// docLedger is what the agent read in this session, kept across turns.
type docLedger struct {
	mu     sync.Mutex
	texts  []string                 // lowercased, oldest first
	size   int                      // total length of texts
	pages  map[string]*docPageState // WebFetch pages read in parts
	usedOK map[string]bool          // options of commands that succeeded
}

type docPageState struct {
	seen  map[int]bool
	total int
}

func (d *docLedger) add(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	t := strings.ToLower(text)
	d.texts = append(d.texts, t)
	d.size += len(t)
	for d.size > docCorpusMax && len(d.texts) > 1 {
		d.size -= len(d.texts[0])
		d.texts = d.texts[1:]
	}
}

func (d *docLedger) has(word string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.usedOK[word] {
		return true
	}
	for _, t := range d.texts {
		if strings.Contains(t, word) {
			return true
		}
	}
	return false
}

func (d *docLedger) markPart(url string, part, total int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pages == nil {
		d.pages = map[string]*docPageState{}
	}
	p := d.pages[url]
	if p == nil {
		p = &docPageState{seen: map[int]bool{}}
		d.pages[url] = p
	}
	p.seen[part], p.total = true, total
}

// unfinished lists pages read only in part, with the offset where reading continues.
func (d *docLedger) unfinished() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	var out []string
	for url, p := range d.pages {
		if p.total <= 1 || p.total > docMustFinish || len(p.seen) >= p.total {
			continue
		}
		next := 0
		for p.seen[next] {
			next++
		}
		out = append(out, fmt.Sprintf("%s (read %d of %d parts; continue with offset=%d)", url, len(p.seen), p.total, next*docPageSize))
	}
	sort.Strings(out)
	return out
}

func (d *docLedger) succeeded(flags []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.usedOK == nil {
		d.usedOK = map[string]bool{}
	}
	for _, f := range flags {
		d.usedOK[f] = true
	}
}

// ---------------------------------------------------------------- commands

// everyday tooling whose options are common knowledge; the gate looks at specialized programs
var commonPrograms = map[string]bool{}

func init() {
	for _, p := range strings.Fields(`git go gofmt npm npx pnpm yarn node deno bun python python3 py pip pip3 uv cargo rustc rustup
		ls dir cd pwd cat head tail less more grep egrep rg find fd sed awk sort uniq wc cut tr tee xargs echo printf test true false
		mkdir rmdir rm cp mv ln touch chmod chown stat du df file which where whereis type env export set unset source sleep date
		curl wget tar zip unzip gzip gunzip diff patch make cmake jq yq ps kill pkill taskkill tasklist powershell pwsh cmd bash sh
		code tsc eslint prettier vite vitest jest mocha pytest ruff black mypy dotnet java javac mvn gradle docker kubectl
		gh claude codex infuse open start explorer clip`) {
		commonPrograms[p] = true
	}
}

// genericFlags are conventions shared by nearly every CLI.
var genericFlags = map[string]bool{"help": true, "version": true, "verbose": true, "quiet": true, "silent": true, "force": true, "yes": true, "dry-run": true, "no-color": true, "color": true, "debug": true, "output": true, "json": true, "recursive": true, "all": true, "config": true, "port": true, "host": true}

var reLongFlag = regexp.MustCompile(`(?:^|[\s"'=,;(])--([a-zA-Z][a-zA-Z0-9_-]*[a-zA-Z0-9])`)

// shellWords splits a command into words, honoring quotes; operators (| & ; && ||) are their own words.
func shellWords(cmd string) []string {
	var out []string
	var cur strings.Builder
	var q rune
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range cmd {
		switch {
		case q != 0:
			if r == q {
				q = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			q = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		case r == '|' || r == '&' || r == ';':
			flush()
			out = append(out, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

type cmdSegment struct {
	program string   // base name, lowercase, without extension
	path    string   // as written
	flags   []string // long options, without dashes
	files   []string // words that look like paths
}

// commandSegments splits a command line into its programs and their long options. Options written in
// an environment assignment (ARGS=--x) belong to the segment's program.
func commandSegments(cmd string) []cmdSegment {
	var segs []cmdSegment
	var cur *cmdSegment
	wrappers := map[string]bool{"sudo": true, "env": true, "time": true, "nohup": true, "exec": true, "xargs": true, "npx": true, "call": true, "&": true}
	for _, w := range shellWords(cmd) {
		if w == "|" || w == "&" || w == ";" {
			cur = nil
			continue
		}
		if cur == nil {
			segs = append(segs, cmdSegment{})
			cur = &segs[len(segs)-1]
		}
		for _, m := range reLongFlag.FindAllStringSubmatch(" "+w, -1) {
			cur.flags = append(cur.flags, strings.ToLower(m[1]))
		}
		if cur.program == "" {
			if i := strings.Index(w, "="); i > 0 && !strings.ContainsAny(w[:i], `/\.`) { // VAR=value
				continue
			}
			base := strings.ToLower(filepath.Base(strings.ReplaceAll(w, `\`, "/")))
			for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
				base = strings.TrimSuffix(base, ext)
			}
			if wrappers[base] {
				continue
			}
			cur.program, cur.path = base, w
			continue
		}
		if strings.ContainsAny(w, `/\`) || strings.Contains(w, ".") {
			cur.files = append(cur.files, w)
		}
	}
	return segs
}

// helpCommand reports whether a command only reads documentation or files (its output is research).
func helpCommand(cmd string) bool {
	c := " " + strings.ToLower(cmd) + " "
	if strings.Contains(c, " --help") || strings.Contains(c, " -h ") || strings.Contains(c, " /? ") || strings.Contains(c, " help ") {
		return true
	}
	for _, p := range []string{" man ", " curl ", " wget ", " cat ", " head ", " tail ", " less ", " grep ", " rg ", " ls ", " info "} {
		if strings.HasPrefix(c, p) {
			return true
		}
	}
	return false
}

// commandDocCheck returns why a command may not run yet ("" when it may).
func (s *Session) commandDocCheck(cmd string) string {
	if !helpCommand(cmd) {
		if open := s.docs.unfinished(); len(open) > 0 {
			return "[infuse] Command refused: some documentation was read only in part. Read it to the end before acting (WebFetch with offset):\n- " + strings.Join(open, "\n- ")
		}
	}
	userText := strings.ToLower(s.userWords())
	var missing []string
	var programs []string
	for _, seg := range commandSegments(cmd) {
		if seg.program == "" || commonPrograms[seg.program] || s.projectProgram(seg.path) {
			continue
		}
		var miss []string
		for _, f := range seg.flags {
			if genericFlags[f] || strings.Contains(userText, f) || s.docs.has(f) || s.docs.has(strings.ReplaceAll(f, "-", "_")) || s.inProjectFiles(f, seg.files) {
				continue
			}
			miss = append(miss, "--"+f)
		}
		if len(miss) > 0 {
			missing = append(missing, miss...)
			programs = append(programs, seg.program)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return fmt.Sprintf("[infuse] Command refused: it uses options of %s that appear in nothing you have read in this session: %s.\nRead the official documentation that describes each one — the whole page (WebFetch, every part), the output of `%s --help`, or the program's source code — and only then run it. An option counts when it is in text you actually read, not in your memory.",
		strings.Join(dedupe(programs), ", "), strings.Join(dedupe(missing), ", "), programs[0])
}

// projectProgram reports whether a program is the project's own (its source is its documentation).
func (s *Session) projectProgram(path string) bool {
	if !strings.ContainsAny(path, `/\`) {
		return false
	}
	p := FromMsysPath(path)
	if !filepath.IsAbs(p) {
		p = filepath.Join(s.Cwd, p)
	}
	return within(s.Root, p)
}

// inProjectFiles reports whether the option is written in a file the command names or in the
// project's scripts (package.json, Makefile, README) — options the project itself defines or documents.
func (s *Session) inProjectFiles(flag string, files []string) bool {
	cands := []string{"package.json", "Makefile", "README.md", "justfile", "Taskfile.yml"}
	for _, f := range files {
		cands = append(cands, FromMsysPath(f))
	}
	for _, f := range cands {
		p := f
		if !filepath.IsAbs(p) {
			p = filepath.Join(s.Cwd, p)
		}
		if !within(s.Root, p) {
			continue
		}
		if fi, err := os.Stat(p); err != nil || fi.IsDir() || fi.Size() > 2<<20 {
			continue
		}
		if b, err := os.ReadFile(p); err == nil && strings.Contains(strings.ToLower(string(b)), flag) {
			return true
		}
	}
	return false
}

// userWords is what the user wrote in this session (options the user asked for are theirs to choose).
func (s *Session) userWords() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b strings.Builder
	for _, m := range s.Messages {
		if m.Role != "user" {
			continue
		}
		for _, c := range m.Content {
			if c.Type == "text" && !strings.HasPrefix(c.Text, "[infuse]") && !strings.HasPrefix(c.Text, "[hook") {
				b.WriteString(c.Text + "\n")
			}
		}
	}
	return b.String()
}

// noteRead keeps the text a research call delivered, for commandDocCheck.
func (s *Session) noteRead(name string, input json.RawMessage, out string, isErr bool) {
	if isErr {
		return
	}
	switch {
	case name == "WebFetch", name == "Skill", name == "Agent", strings.HasPrefix(name, "mcp__"):
	case name == "Browser":
		var in struct{ Action string }
		_ = json.Unmarshal(input, &in)
		if in.Action != "text" && in.Action != "dom" {
			return
		}
	case name == "Read":
		if !isSourceRead(input) {
			return
		}
	case name == "Bash":
		var in struct{ Command string }
		_ = json.Unmarshal(input, &in)
		if !helpCommand(in.Command) {
			return
		}
	default:
		return
	}
	s.docs.add(out)
}

func dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
