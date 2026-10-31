package main

// Agent tools. Every tool that touches the project goes through the filter by construction:
// protected paths are refused here, writes are journaled and checkpointed, outputs come back infused.
// Adding a tool = registering an implementation.

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ToolContext struct {
	S     *Session
	Root  string
	Cwd   string
	Rules *RuleSet
}

type Tool interface {
	Spec() ToolSpec
	Category() string // read | write | exec | net | ui
	Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool)
}

var (
	toolRegistry = map[string]Tool{}
	toolOrder    []string
)

func registerTool(t Tool) {
	toolRegistry[t.Spec().Name] = t
	toolOrder = append(toolOrder, t.Spec().Name)
}

func toolSpecs() []ToolSpec {
	out := make([]ToolSpec, 0, len(toolOrder))
	for _, n := range toolOrder {
		out = append(out, toolRegistry[n].Spec())
	}
	return out
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

const maxToolOutput = 40000

func clip(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	return s[:maxToolOutput/2] + fmt.Sprintf("\n\n[... %d characters omitted ...]\n\n", len(s)-maxToolOutput) + s[len(s)-maxToolOutput/2:]
}

// resolvePath turns a tool path into an absolute path inside the project; refuses escapes and protected paths.
func (tc *ToolContext) resolvePath(p string, forWrite bool) (string, error) {
	p = FromMsysPath(p)
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(tc.Cwd, p)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(tc.Root, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		if forWrite {
			return "", fmt.Errorf("path outside the project: %s", p)
		}
	}
	if !forWrite && err == nil && !strings.HasPrefix(rel, "..") && hiddenInternal(rel) {
		return "", fmt.Errorf("%s is an internal infuse file (sessions, journal, history), not part of the project. From .infuse/ the agent reads only the pinned documentation (.infuse/docs), the conventions, the map (.infuse/map/MAP.md) and its own captures", toPosix(rel))
	}
	if forWrite && tc.S.isProtected(abs) {
		Journal(tc.Root, JournalEntry{Kind: "blocked", Path: toPosix(rel), Detail: "tentativa de escrita em caminho protegido"})
		Lesson(tc.Root, "protected-path-write", "tentativa de escrever em "+toPosix(rel))
		return "", fmt.Errorf("protected path, read-only for the agent: %s. The oracle and the protected paths are set by the user and cannot be changed by the agent", toPosix(rel))
	}
	return abs, nil
}

func (tc *ToolContext) afterWrite(abs string) []string {
	rel := relTo(tc.Root, abs)
	st, _ := os.Stat(abs)
	size := 0
	if st != nil {
		size = int(st.Size())
	}
	Journal(tc.Root, JournalEntry{Kind: "write", Path: rel, Bytes: size, Detail: "agente"})
	tc.S.scheduleMapRefresh()
	var lines []string
	if m := LoadMap(tc.Root); m != nil {
		if d := DescribeDependents(m, rel); d != "" {
			lines = append(lines, d)
		}
	}
	lines = append(lines, ApplyWriteRules(tc.Root, tc.Rules, []string{abs})...)
	if tc.S.settings.Diagnostics == nil || *tc.S.settings.Diagnostics {
		if d := fileDiagnostics(tc.Root, abs); d != "" {
			lines = append(lines, fmt.Sprintf("[diagnostics] problems found by the project's checker in this file:\n%s", d))
		}
	}
	return lines
}

// ---------------------------------------------------------------- Bash

type bashTool struct{}

func (bashTool) Category() string { return "exec" }
func (bashTool) Spec() ToolSpec {
	return ToolSpec{Name: "Bash", Description: "Runs a command in the project's shell (bash). Use it to run builds, tests, git and utilities. The output comes back with project information when relevant. run_in_background=true for something that keeps running (server, game, watcher): returns at once with the id; read the output with BashOutput and stop it with KillShell. You are notified when the process ends.",
		InputSchema: schema(`{"type":"object","properties":{"command":{"type":"string","description":"Command to run"},"timeout":{"type":"integer","description":"Timeout in seconds (default 120, maximum 600)"},"description":{"type":"string","description":"Short description of what the command does"},"run_in_background":{"type":"boolean","description":"runs without waiting for it to finish"}},"required":["command"]}`)}
}

func (bashTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Command     string `json:"command"`
		Timeout     int    `json:"timeout"`
		Description string `json:"description"`
		Background  bool   `json:"run_in_background"`
	}
	_ = json.Unmarshal(input, &in)
	if strings.TrimSpace(in.Command) == "" {
		return "empty command", true
	}
	if in.Timeout <= 0 {
		in.Timeout = 120
	}
	if in.Timeout > 600 {
		in.Timeout = 600
	}
	started := time.Now().UnixMilli()
	// The guard runs first: protected targets are refused or neutralized before the transform can write.
	guarded, guardNotes, gerr := newGuard(tc.Root, tc.S.settings.ProtectedPaths).GuardCommand(in.Command, tc.Cwd)
	if gerr != nil {
		return gerr.Error(), true
	}
	in.Command = guarded
	// Heredoc file writes happen inside TransformCommand: checkpoint their targets first.
	for _, h := range FindHeredocs(in.Command) {
		_, _, line := CommandLineOf(in.Command, h)
		if intent := DetectFileWrite(line); intent != nil {
			tc.S.checkpointBeforeWrite(resolveIn(tc.Cwd, FromMsysPath(intent.Path)))
		}
	}
	tr := TransformCommand(in.Command, tc.Cwd)
	if tc.Rules.Disabled["heredoc"] {
		tr = TransformResult{Command: in.Command}
	}
	command, ruleLines := ApplyCommandRules(tc.Root, tc.Rules, tr.Command)
	Journal(tc.Root, JournalEntry{Kind: "command", Command: in.Command, Detail: "agente"})
	for _, a := range tr.Actions {
		if a.Kind == ActLesson && a.Vice != "" {
			Lesson(tc.Root, a.Vice, a.Detail)
		} else {
			Journal(tc.Root, JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
	if in.Background {
		p, err := tc.S.startBackground(command, in.Description, tc.Cwd)
		if err != nil {
			return fmt.Sprintf("did not start: %s", err.Error()), true
		}
		// the first moments of output: a command that fails at once says so now
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && ctx.Err() == nil {
			if tc.S.proc(p.ID).State != "running" {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		first, off := p.out.since(0)
		tc.S.mu.Lock()
		p.read = off
		state := p.State
		tc.S.mu.Unlock()
		msg := fmt.Sprintf("process %s running in the background (pid %d). Read the new output with BashOutput(id=%q); stop it with KillShell. The user follows it in the Processes pane.", p.ID, p.PID, p.ID)
		if state != "running" {
			msg = fmt.Sprintf("process %s in the background (pid %d) has already finished. Read the new output with BashOutput(id=%q); stop it with KillShell. The user follows it in the Processes pane.", p.ID, p.PID, p.ID)
		}
		if strings.TrimSpace(first) != "" {
			msg += fmt.Sprintf("\nFirst 2 s of output:\n%s", clip(first))
		}
		return msg, state != "running" && p.Exit != nil && *p.Exit != 0
	}
	// Track the working directory across commands, as a human shell would.
	wrapped := command + "\nprintf '\\n__INFUSE_CWD__%s' \"$PWD\""
	cctx, cancel := context.WithTimeout(ctx, time.Duration(in.Timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cctx, FindRealBash(), "-c", wrapped)
	noConsoleWindow(cmd)
	cmd.Dir = tc.Cwd
	cmd.Env = append(os.Environ(), "INFUSE_AGENT=1", "GIT_TERMINAL_PROMPT=0", "CI=1")
	buf := newCapWriter(1 << 20) // head and tail kept: unbounded output cannot exhaust memory
	cmd.Stdout, cmd.Stderr = buf, buf
	err := cmd.Run()
	out := buf.String()
	if i := strings.LastIndex(out, "__INFUSE_CWD__"); i >= 0 {
		if nc := strings.TrimSpace(out[i+len("__INFUSE_CWD__"):]); nc != "" {
			if abs := FromMsysPath(nc); within(tc.Root, abs) {
				tc.S.setCwd(abs)
			}
		}
		out = strings.TrimRight(out[:i], "\n")
	}
	isErr := false
	if cctx.Err() == context.DeadlineExceeded {
		out += fmt.Sprintf("\n[timed out after %ds]", in.Timeout)
		isErr = true
	} else if err != nil {
		out += fmt.Sprintf("\n[exit %d]", exitCode(err))
		isErr = true
	}
	trailer := buildTrailer(tc.Root, tc.Cwd, in.Command, tr.Actions, started, tc.Rules, append(guardNotes, ruleLines...))
	if trailer != "" {
		out = strings.TrimRight(out, "\n") + "\n\n" + trailer
	}
	return clip(out), isErr
}

// ---------------------------------------------------------------- Read

type readTool struct{}

func (readTool) Category() string { return "read" }
func (readTool) Spec() ToolSpec {
	return ToolSpec{Name: "Read", Description: "Reads a project file with line numbers. Use offset/limit for large files.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"offset":{"type":"integer","description":"First line (1-based)"},"limit":{"type":"integer","description":"Number of lines"}},"required":["path"]}`)}
}

func (readTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path          string `json:"path"`
		Offset, Limit int
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if mt := imageType(abs); mt != "" { // the model sees the image itself
		if len(b) > 5<<20 {
			return fmt.Sprintf("image too large to send to the model (%d bytes; maximum 5 MB)", len(b)), true
		}
		tc.S.mu.Lock()
		tc.S.pendingImages = append(tc.S.pendingImages, Block{Type: "image", MediaType: mt, Data: base64.StdEncoding.EncodeToString(b)})
		tc.S.mu.Unlock()
		tc.S.markRead(abs)
		return fmt.Sprintf("image %s (%d bytes) attached to this reply for you to see", relTo(tc.Root, abs), len(b)), false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return fmt.Sprintf("binary file (%d bytes)", len(b)), true
	}
	lines := strings.Split(string(b), "\n")
	start := in.Offset
	if start < 1 {
		start = 1
	}
	end := len(lines)
	if in.Limit > 0 && start-1+in.Limit < end {
		end = start - 1 + in.Limit
	}
	if start > len(lines) {
		return "", false
	}
	var sb strings.Builder
	shown := start - 1
	for i := start - 1; i < end; i++ {
		if sb.Len() > maxToolOutput-2000 { // a long file is read in parts, never with its middle cut out
			break
		}
		fmt.Fprintf(&sb, "%6d\t%s\n", i+1, lines[i])
		shown = i + 1
	}
	if shown < end {
		fmt.Fprintf(&sb, "\n[continues: the file has %d lines; read on from here with Read offset=%d]\n", len(lines), shown+1)
	}
	tc.S.noteCoverage(abs, len(lines), start, shown)
	tc.S.markRead(abs)
	out := sb.String()
	if m := LoadMap(tc.Root); m != nil {
		if d := DescribeDependents(m, relTo(tc.Root, abs)); d != "" {
			out += "\n" + d
		}
	}
	return clip(out), false
}

// ---------------------------------------------------------------- Write

type writeTool struct{}

func (writeTool) Category() string { return "write" }
func (writeTool) Spec() ToolSpec {
	return ToolSpec{Name: "Write", Description: "Creates or overwrites a file with its complete content. For targeted changes to an existing file, prefer Edit.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)}
}

func (writeTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path, Content string }
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	tc.S.checkpointBeforeWrite(abs)
	_ = os.MkdirAll(filepath.Dir(abs), 0o755)
	if err := os.WriteFile(abs, []byte(in.Content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("File written: %s (%d bytes)", relTo(tc.Root, abs), len(in.Content))}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// ---------------------------------------------------------------- Edit

type editTool struct{}

func (editTool) Category() string { return "write" }
func (editTool) Spec() ToolSpec {
	return ToolSpec{Name: "Edit", Description: "Replaces an exact passage of a file. old_string must occur exactly once (or use replace_all). Read the file before editing it.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"]}`)}
}

func (editTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path       string `json:"path"`
		OldString  string `json:"old_string"`
		NewString  string `json:"new_string"`
		ReplaceAll bool   `json:"replace_all"`
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if !tc.S.wasRead(abs) {
		return "read the file with Read before editing it", true
	}
	content := string(b)
	n := strings.Count(content, in.OldString)
	if in.OldString == "" || n == 0 {
		return "old_string not found in the file", true
	}
	if n > 1 && !in.ReplaceAll {
		return fmt.Sprintf("old_string occurs %d times; make it unique or use replace_all", n), true
	}
	tc.S.checkpointBeforeWrite(abs)
	next := strings.Replace(content, in.OldString, in.NewString, map[bool]int{true: -1, false: 1}[in.ReplaceAll])
	if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("Edited: %s (%d replacement(s))", relTo(tc.Root, abs), map[bool]int{true: n, false: 1}[in.ReplaceAll])}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// ---------------------------------------------------------------- MultiEdit

type multiEditTool struct{}

func (multiEditTool) Category() string { return "write" }
func (multiEditTool) Spec() ToolSpec {
	return ToolSpec{Name: "MultiEdit", Description: "Applies several exact replacements to the same file, in order, all at once: either all of them apply or none is written. Each old_string must occur exactly once in the text as it stands after the previous edits (or use replace_all). Read the file first.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"},"edits":{"type":"array","items":{"type":"object","properties":{"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["old_string","new_string"]}}},"required":["path","edits"]}`)}
}

func (multiEditTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Path  string `json:"path"`
		Edits []struct {
			OldString  string `json:"old_string"`
			NewString  string `json:"new_string"`
			ReplaceAll bool   `json:"replace_all"`
		} `json:"edits"`
	}
	_ = json.Unmarshal(input, &in)
	abs, err := tc.resolvePath(in.Path, true)
	if err != nil {
		return err.Error(), true
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return err.Error(), true
	}
	if !tc.S.wasRead(abs) {
		return "read the file with Read before editing it", true
	}
	if len(in.Edits) == 0 {
		return "no edits", true
	}
	content := string(b)
	total := 0
	for i, e := range in.Edits {
		n := strings.Count(content, e.OldString)
		if e.OldString == "" || n == 0 {
			return fmt.Sprintf("edit %d: old_string not found (nothing was written)", i+1), true
		}
		if n > 1 && !e.ReplaceAll {
			return fmt.Sprintf("edit %d: old_string occurs %d times; make it unique or use replace_all (nothing was written)", i+1, n), true
		}
		if e.ReplaceAll {
			content = strings.ReplaceAll(content, e.OldString, e.NewString)
			total += n
		} else {
			content = strings.Replace(content, e.OldString, e.NewString, 1)
			total++
		}
	}
	tc.S.checkpointBeforeWrite(abs)
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return err.Error(), true
	}
	lines := append([]string{fmt.Sprintf("Edited %s: %d edits, %d replacement(s)", relTo(tc.Root, abs), len(in.Edits), total)}, tc.afterWrite(abs)...)
	return strings.Join(lines, "\n"), false
}

// imageType is the media type of an image the model can see, by extension.
func imageType(p string) string {
	switch strings.ToLower(filepath.Ext(p)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// ---------------------------------------------------------------- Glob

type globTool struct{}

func (globTool) Category() string { return "read" }
func (globTool) Spec() ToolSpec {
	return ToolSpec{Name: "Glob", Description: "Finds files by pattern (e.g. src/**/*.ts). Sorted by modification time, most recent first.",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string","description":"Base folder (default: the project root)"}},"required":["pattern"]}`)}
}

func globToRegex(pat string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch {
		case c == '*' && i+1 < len(pat) && pat[i+1] == '*':
			b.WriteString(".*")
			i++
			if i+1 < len(pat) && pat[i+1] == '/' {
				i++
				b.WriteString("(?:/|)")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			j := strings.IndexByte(pat[i:], '}')
			if j > 0 {
				alts := strings.Split(pat[i+1:i+j], ",")
				for k := range alts {
					alts[k] = regexp.QuoteMeta(alts[k])
				}
				b.WriteString("(?:" + strings.Join(alts, "|") + ")")
				i += j
				continue
			}
			b.WriteString(`\{`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile("^$")
	}
	return re
}

func (globTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Pattern, Path string }
	_ = json.Unmarshal(input, &in)
	base := tc.Root
	if in.Path != "" {
		if abs, err := tc.resolvePath(in.Path, false); err == nil {
			base = abs
		}
	}
	re := globToRegex(strings.TrimPrefix(in.Pattern, "./"))
	type hit struct {
		p string
		t time.Time
	}
	var hits []hit
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(base, p)
		if re.MatchString(toPosix(rel)) {
			info, _ := d.Info()
			t := time.Time{}
			if info != nil {
				t = info.ModTime()
			}
			hits = append(hits, hit{toPosix(rel), t})
		}
		if len(hits) >= 2000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Slice(hits, func(i, j int) bool { return hits[i].t.After(hits[j].t) })
	var sb strings.Builder
	for _, h := range hits {
		sb.WriteString(h.p + "\n")
	}
	if len(hits) == 0 {
		return "no files found", false
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- Grep

type grepTool struct{}

func (grepTool) Category() string { return "read" }
func (grepTool) Spec() ToolSpec {
	return ToolSpec{Name: "Grep", Description: "Searches file contents with a regular expression. Returns file:line:text.",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string","description":"Filter files, e.g. *.ts"},"ignore_case":{"type":"boolean"},"context":{"type":"integer","description":"Lines of context"},"files_only":{"type":"boolean"}},"required":["pattern"]}`)}
}

func (grepTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		IgnoreCase bool   `json:"ignore_case"`
		Context    int    `json:"context"`
		FilesOnly  bool   `json:"files_only"`
	}
	_ = json.Unmarshal(input, &in)
	pat := in.Pattern
	if in.IgnoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return fmt.Sprintf("invalid regex: %s", err.Error()), true
	}
	base := tc.Root
	if in.Path != "" {
		if abs, err := tc.resolvePath(in.Path, false); err == nil {
			base = abs
		}
	}
	var fileRe *regexp.Regexp
	if in.Glob != "" {
		fileRe = globToRegex("**/" + in.Glob)
	}
	var sb strings.Builder
	matches := 0
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || matches > 500 {
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(tc.Root, p)
		rel = toPosix(rel)
		if fileRe != nil && !fileRe.MatchString(rel) {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 2_000_000 || bytes.IndexByte(b, 0) >= 0 {
			return nil
		}
		lines := strings.Split(string(b), "\n")
		fileHit := false
		for i, l := range lines {
			if !re.MatchString(l) {
				continue
			}
			matches++
			if in.FilesOnly {
				if !fileHit {
					sb.WriteString(rel + "\n")
				}
				fileHit = true
				continue
			}
			from, to := i-in.Context, i+in.Context
			if from < 0 {
				from = 0
			}
			if to >= len(lines) {
				to = len(lines) - 1
			}
			for j := from; j <= to; j++ {
				sep := "-"
				if j == i {
					sep = ":"
				}
				fmt.Fprintf(&sb, "%s:%d%s%s\n", rel, j+1, sep, lines[j])
			}
			if in.Context > 0 {
				sb.WriteString("--\n")
			}
		}
		return nil
	})
	if matches == 0 {
		return "no matches", false
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- LS

type lsTool struct{}

func (lsTool) Category() string { return "read" }
func (lsTool) Spec() ToolSpec {
	return ToolSpec{Name: "LS", Description: "Lists a project folder. The answer includes the folder's map: what each file exports, what depends on it, files that are too large.",
		InputSchema: schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":[]}`)}
}

func (lsTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct{ Path string }
	_ = json.Unmarshal(input, &in)
	if in.Path == "" {
		in.Path = "."
	}
	abs, err := tc.resolvePath(in.Path, false)
	if err != nil {
		return err.Error(), true
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return err.Error(), true
	}
	var sb strings.Builder
	here := relTo(tc.Root, abs)
	for _, e := range entries {
		if hiddenInternal(filepath.ToSlash(filepath.Join(here, e.Name()))) {
			continue // infuse's own records are not listed as part of the project
		}
		if e.IsDir() {
			sb.WriteString(e.Name() + "/\n")
		} else {
			sb.WriteString(e.Name() + "\n")
		}
	}
	rel := relTo(tc.Root, abs)
	if rel == "" {
		rel = "."
	}
	m := LoadOrBuildMap(tc.Root)
	if d := DescribeDir(m, rel); d != "" {
		sb.WriteString("\n" + d + "\n")
	}
	for _, l := range ApplyListRules(tc.Root, tc.Rules, rel) {
		sb.WriteString(l + "\n")
	}
	return clip(sb.String()), false
}

// ---------------------------------------------------------------- WebFetch

type webFetchTool struct{}

func (webFetchTool) Category() string { return "net" }
func (webFetchTool) Spec() ToolSpec {
	return ToolSpec{Name: "WebFetch", Description: "Downloads a web page or document and returns its text (HTML converted to text), in parts of 30000 characters. A page in parts must be read to the end (offset) before you run commands or change files.",
		InputSchema: schema(`{"type":"object","properties":{"url":{"type":"string"},"offset":{"type":"integer","description":"where to continue reading (given at the end of each part)"}},"required":["url"]}`)}
}

var reTag = regexp.MustCompile(`(?s)<(script|style)[^>]*>.*?</(script|style)>|<[^>]+>`)
var reWs = regexp.MustCompile(`\n\s*\n+`)

func (webFetchTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		URL    string
		Offset int
	}
	_ = json.Unmarshal(input, &in)
	if bad, why := privateTarget(in.URL); bad { // no requests into this machine or the private network
		Journal(tc.Root, JournalEntry{Kind: "blocked", Command: in.URL, Detail: "WebFetch recusado: " + why})
		return fmt.Sprintf("WebFetch refused: %s. The agent only fetches public addresses.", why), true
	}
	Journal(tc.Root, JournalEntry{Kind: "fetch", Command: in.URL, Detail: "pesquisa"})
	req, err := http.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return err.Error(), true
	}
	req.Header.Set("User-Agent", "infuse/"+version)
	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return err.Error(), true
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	text := string(b)
	if strings.Contains(resp.Header.Get("content-type"), "html") {
		text = reTag.ReplaceAllString(text, " ")
		text = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(text)
		text = reWs.ReplaceAllString(text, "\n\n")
	}
	text = strings.TrimSpace(text)
	total := max(1, (len(text)+docPageSize-1)/docPageSize)
	part := min(max(in.Offset, 0)/docPageSize, total-1)
	body := text[part*docPageSize : min(len(text), (part+1)*docPageSize)]
	head := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if total > 1 {
		head = fmt.Sprintf("HTTP %d · part %d of %d", resp.StatusCode, part+1, total)
		if part+1 < total {
			body += fmt.Sprintf("\n\n[continues: WebFetch with offset=%d — read to the end before acting]", (part+1)*docPageSize)
		}
	}
	if tc.S != nil && resp.StatusCode < 400 {
		tc.S.docs.markPart(in.URL, part, total)
	}
	return externalContent(in.URL, head+"\n\n"+body), resp.StatusCode >= 400
}

// ---------------------------------------------------------------- TodoWrite

type TaskItem struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Status string `json:"status"`          // pending | in_progress | done | blocked
	Proof  string `json:"proof,omitempty"` // how the task will be shown to work (a command, a page, a file)
	Note   string `json:"note,omitempty"`  // set by infuse, never by the model (why a task was closed as blocked)
}

type todoTool struct{}

func (todoTool) Category() string { return "ui" }
func (todoTool) Spec() ToolSpec {
	return ToolSpec{Name: "TodoWrite", Description: "The plan for the request, visible to the user and checked off in real time. Required before changing files. Send the complete list on every update; each open task with a proof (how it will be proven). Status: pending, in_progress, done, blocked (with the reason in the Report). The turn only ends with all of them done or blocked.",
		InputSchema: schema(`{"type":"object","properties":{"tasks":{"type":"array","items":{"type":"object","properties":{"id":{"type":"string"},"text":{"type":"string"},"status":{"type":"string","enum":["pending","in_progress","done","blocked"]},"proof":{"type":"string","description":"how this task will be proven: the command, the page or the file that shows it works"}},"required":["id","text","status"]}}},"required":["tasks"]}`)}
}

func (todoTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Tasks []TaskItem `json:"tasks"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return err.Error(), true
	}
	if len(in.Tasks) == 0 {
		return "the plan needs at least one task", true
	}
	var noProof []string
	for _, t := range in.Tasks {
		if t.Status != "done" && t.Status != "blocked" && strings.TrimSpace(t.Proof) == "" {
			noProof = append(noProof, t.Text)
		}
	}
	if len(noProof) > 0 { // the protocol: every open task says how it will be proven
		return fmt.Sprintf("plan refused: every open task needs a proof (the command, the page or the file that will prove it works). Without proof: %s", strings.Join(noProof, "; ")), true
	}
	for i := range in.Tasks {
		in.Tasks[i].Note = ""
	}
	tc.S.setTasks(in.Tasks)
	tc.S.mu.Lock()
	tc.S.gate.planned = true
	tc.S.mu.Unlock()
	return fmt.Sprintf("plan recorded (%d tasks); the user sees each one checked off as you update it", len(in.Tasks)), false
}

// ---------------------------------------------------------------- AskUserQuestion

type askTool struct{}

func (askTool) Category() string { return "ui" }
func (askTool) Spec() ToolSpec {
	return ToolSpec{Name: "AskUserQuestion", Description: "Asks the user a question in the interface, with options. Use it when a decision belongs to the user and cannot be inferred.",
		InputSchema: schema(`{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}}},"required":["question"]}`)}
}

func (askTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	_ = json.Unmarshal(input, &in)
	answer, ok := tc.S.askUser(ctx, in.Question, in.Options)
	if !ok {
		return "the user did not answer", true
	}
	return fmt.Sprintf("User's answer: %s", answer), false
}

func init() {
	for _, t := range []Tool{bashTool{}, readTool{}, writeTool{}, editTool{}, multiEditTool{}, globTool{}, grepTool{}, lsTool{}, webFetchTool{}, todoTool{}, askTool{}} {
		registerTool(t)
	}
}
