package main

// The protocol: what the agent must do is decided by infuse, not by the model. Every rule here is
// checked by code on each tool call or at the end of each turn, and an action outside the protocol is
// refused with the reason — the model's preference does not enter.
//
//   Before a write:  a plan registered in this turn (TodoWrite, each open task with its proof);
//                    the docs of every library or external API the new code starts to use, consulted in
//                    this turn (web, official docs, pinned docs, the package's own source);
//                    the project's conventions (indentation, quotes, semicolons, modules, file names);
//                    the file-size limit.
//   Before the turn ends: no open plan task; no checker error in a changed file; a checked Report;
//                    the mandatory independent review answered.

import (
	"context"
	"encoding/json"
	"infuse/internal/knowledge"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ---------------------------------------------------------------- research tracking

// noteResearch records what a research call looked at (queries, addresses, files), for the
// "docs before use" rule.
func (s *Session) noteResearch(name string, input json.RawMessage) {
	var in struct{ Query, URL, Path, Name, Prompt, Action string }
	_ = json.Unmarshal(input, &in)
	text := strings.ToLower(strings.Join([]string{in.Query, in.URL, filepath.ToSlash(in.Path), in.Name, in.Prompt}, " "))
	if strings.TrimSpace(text) == "" {
		return
	}
	s.mu.Lock()
	s.gate.researched = append(s.gate.researched, text)
	s.mu.Unlock()
}

// researchedFor reports whether name (a package or a host) was consulted in this turn or is pinned.
func (s *Session) researchedFor(name string) bool {
	n := strings.ToLower(name)
	alt := strings.ReplaceAll(n, "-", " ")
	s.mu.Lock()
	seen := append([]string{}, s.gate.researched...)
	s.mu.Unlock()
	for _, t := range seen {
		if strings.Contains(t, n) || strings.Contains(t, alt) {
			return true
		}
	}
	for _, d := range loadDocs(s.Root) {
		if strings.Contains(strings.ToLower(d.URL+" "+d.Title+" "+d.File), n) {
			return true
		}
	}
	return false
}

// isSourceRead reports whether a Read looks at documentation or a package's own source (not project code).
func isSourceRead(input json.RawMessage) bool {
	var in struct{ Path string }
	_ = json.Unmarshal(input, &in)
	p := strings.ToLower(filepath.ToSlash(in.Path))
	for _, k := range []string{"node_modules/", "/go/pkg/mod/", "site-packages/", ".infuse/docs/", "/.cargo/registry/"} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- dependencies a piece of code uses

var nodeBuiltins = map[string]bool{"assert": true, "async_hooks": true, "buffer": true, "child_process": true, "cluster": true, "console": true, "constants": true, "crypto": true, "dgram": true, "diagnostics_channel": true, "dns": true, "domain": true, "events": true, "fs": true, "http": true, "http2": true, "https": true, "inspector": true, "module": true, "net": true, "os": true, "path": true, "perf_hooks": true, "process": true, "punycode": true, "querystring": true, "readline": true, "repl": true, "stream": true, "string_decoder": true, "sys": true, "test": true, "timers": true, "tls": true, "trace_events": true, "tty": true, "url": true, "util": true, "v8": true, "vm": true, "wasi": true, "worker_threads": true, "zlib": true}

var pyStdlib = map[string]bool{"abc": true, "argparse": true, "array": true, "ast": true, "asyncio": true, "base64": true, "bisect": true, "calendar": true, "collections": true, "concurrent": true, "configparser": true, "contextlib": true, "copy": true, "csv": true, "ctypes": true, "dataclasses": true, "datetime": true, "decimal": true, "difflib": true, "email": true, "enum": true, "errno": true, "fnmatch": true, "fractions": true, "functools": true, "gc": true, "getpass": true, "glob": true, "gzip": true, "hashlib": true, "heapq": true, "hmac": true, "html": true, "http": true, "importlib": true, "inspect": true, "io": true, "ipaddress": true, "itertools": true, "json": true, "logging": true, "math": true, "mimetypes": true, "multiprocessing": true, "operator": true, "os": true, "pathlib": true, "pickle": true, "platform": true, "pprint": true, "queue": true, "random": true, "re": true, "secrets": true, "select": true, "shlex": true, "shutil": true, "signal": true, "socket": true, "sqlite3": true, "ssl": true, "statistics": true, "string": true, "struct": true, "subprocess": true, "sys": true, "tarfile": true, "tempfile": true, "textwrap": true, "threading": true, "time": true, "timeit": true, "tkinter": true, "traceback": true, "types": true, "typing": true, "unicodedata": true, "unittest": true, "urllib": true, "uuid": true, "warnings": true, "weakref": true, "xml": true, "zipfile": true, "zlib": true, "__future__": true}

var (
	reJSImport  = regexp.MustCompile(`(?m)(?:\bimport\s+(?:[^'"]*?\s+from\s+)?|\brequire\(\s*|\bimport\(\s*)['"]([^'"]+)['"]`)
	rePyImport  = regexp.MustCompile(`(?m)^\s*(?:from\s+([A-Za-z_][\w.]*)\s+import|import\s+([A-Za-z_][\w.]*))`)
	reGoImport  = regexp.MustCompile(`"([a-z0-9.\-]+\.[a-z]{2,}/[^"\s]+)"`)
	reURLHost   = regexp.MustCompile(`https?://([a-zA-Z0-9.\-]+)`)
	localHosts  = map[string]bool{"localhost": true, "127.0.0.1": true, "0.0.0.0": true, "example.com": true, "example.org": true, "www.example.com": true}
	codeExts    = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true, ".py": true, ".go": true, ".rs": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true, ".vue": true, ".svelte": true, ".css": true, ".scss": true, ".html": true}
	jsLikeExts  = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true}
	reDotsSplit = regexp.MustCompile(`[/.]`)
)

// externalUses lists the packages and hosts that code uses: "pkg:<name>" and "host:<name>".
func externalUses(path, code string) map[string]bool {
	out := map[string]bool{}
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case jsLikeExts[ext] || ext == ".vue" || ext == ".svelte":
		for _, m := range reJSImport.FindAllStringSubmatch(code, -1) {
			spec := m[1]
			if strings.HasPrefix(spec, ".") || strings.HasPrefix(spec, "/") || strings.HasPrefix(spec, "node:") || strings.HasPrefix(spec, "#") {
				continue
			}
			parts := strings.Split(spec, "/")
			name := parts[0]
			if strings.HasPrefix(name, "@") && len(parts) > 1 {
				name += "/" + parts[1]
			}
			if !nodeBuiltins[name] {
				out["pkg:"+name] = true
			}
		}
	case ext == ".py":
		for _, m := range rePyImport.FindAllStringSubmatch(code, -1) {
			mod := firstNonEmpty(m[1], m[2])
			top := strings.Split(mod, ".")[0]
			if top != "" && !pyStdlib[top] {
				out["pkg:"+top] = true
			}
		}
	case ext == ".go":
		for _, m := range reGoImport.FindAllStringSubmatch(code, -1) {
			parts := strings.Split(m[1], "/")
			if len(parts) > 3 {
				parts = parts[:3]
			}
			out["pkg:"+strings.Join(parts, "/")] = true
		}
	}
	if codeExts[ext] {
		for _, m := range reURLHost.FindAllStringSubmatch(code, -1) {
			h := strings.ToLower(m[1])
			if !localHosts[h] && strings.Contains(h, ".") && !strings.HasSuffix(h, ".test") && !strings.HasSuffix(h, ".local") {
				out["host:"+h] = true
			}
		}
	}
	return out
}

// researchName is what must appear in the research for a use (a package's last meaningful part, a host's
// registrable domain).
func researchName(use string) string {
	kind, name, _ := strings.Cut(use, ":")
	if kind == "host" {
		parts := strings.Split(name, ".")
		if len(parts) >= 2 {
			return parts[len(parts)-2] // api.frankfurter.app → frankfurter
		}
		return name
	}
	if strings.Contains(name, "/") && !strings.HasPrefix(name, "@") {
		segs := reDotsSplit.Split(name, -1)
		return segs[len(segs)-1] // github.com/spf13/cobra → cobra
	}
	return strings.TrimPrefix(name, "@")
}

// ---------------------------------------------------------------- the checks before a write

// protocolCheckWrite returns why a write is refused (empty when it follows the protocol).
func (s *Session) protocolCheckWrite(name string, input json.RawMessage) Msg {
	p, before, after, ok := editedContent(s.workRoot(), s.Cwd, name, input)
	if !ok {
		return Msg{}
	}
	rel := filepath.ToSlash(p)
	if strings.Contains(rel, ".infuse/") || strings.HasSuffix(rel, ".md") && !strings.Contains(rel, "/src/") {
		return Msg{} // infuse's own files and prose documents are not code
	}
	// 0. the project's instructions: not edited, references not written, the declared measurements used
	if why := s.instructionWriteCheck(relTo(s.workRoot(), p), before); !why.Empty() {
		return why
	}
	// 1. a plan for this turn
	s.mu.Lock()
	planned := s.gate.planned
	s.mu.Unlock()
	if !planned {
		return M("protocol.no_plan")
	}
	ext := strings.ToLower(filepath.Ext(p))
	// 2. docs before using a library or an external API
	newUses := externalUses(p, after)
	for u := range externalUses(p, before) {
		delete(newUses, u)
	}
	var missing []string
	for u := range newUses {
		if !s.researchedFor(researchName(u)) {
			missing = append(missing, strings.TrimPrefix(strings.TrimPrefix(u, "pkg:"), "host:"))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return M("protocol.docs_first", "uses", strings.Join(missing, ", "))
	}
	// 3. the project's conventions
	if s.settings.EnforceConventions == nil || *s.settings.EnforceConventions {
		if why := conventionViolation(s.Root, p, before, after); !why.Empty() {
			return M("protocol.convention", "why", why)
		}
	}
	// 4. architecture: placement, god functions, tables, styles from tokens, one store, one home per
	// constant, no copies, no import cycles, JSON with its schema (arch*.go)
	if s.settings.EnforceArchitecture == nil || *s.settings.EnforceArchitecture {
		if why := archCheck(s.Root, s.workRoot(), relTo(s.workRoot(), p), before, after); !why.Empty() {
			return M("protocol.architecture", "why", why)
		}
	}
	// 5. size
	limit := s.settings.MaxFileLines
	if limit == 0 {
		limit = 400
	}
	if limit > 0 && codeExts[ext] {
		nb, na := strings.Count(before, "\n")+1, strings.Count(after, "\n")+1
		if before == "" {
			nb = 0
		}
		if na > limit && nb > limit && na > nb { // a file already too big may not grow
			return M("protocol.file_grows", "file", rel, "lines", nb, "limit", limit, "after", na)
		}
		if na > limit && nb <= limit {
			return M("protocol.file_too_big", "file", rel, "lines", na, "limit", limit)
		}
	}
	return Msg{}
}

// ---------------------------------------------------------------- conventions as rules

// ConventionRules is the enforceable part of the learned conventions (with the share of samples).
func indentOf(code string) (string, int) {
	t := Tally{}
	for _, line := range strings.Split(strings.ReplaceAll(code, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || len(trimmed) == len(line) {
			continue
		}
		lead := line[:len(line)-len(trimmed)]
		switch {
		case strings.HasPrefix(lead, "\t"):
			t["tabs"]++
		case len(lead)%4 == 0 && !strings.HasPrefix(strings.TrimSpace(trimmed), "*"):
			t["4"]++
		case len(lead)%2 == 0:
			t["2"]++
		}
	}
	k, n, total := t.Top()
	if total < 5 || n*100/total < 70 {
		return "", total
	}
	if k == "4" && t["2"] == 0 { // all multiples of 4 could still be a 2-space file nested twice
		return "4", total
	}
	if k == "4" {
		return "2", total
	}
	return k, total
}

// conventionViolation compares the written code with the project's strong conventions (≥ 80%).
func conventionViolation(root, path, before, after string) Msg {
	r := loadConventionRules(root)
	if r == nil || r.Files < 3 {
		return Msg{}
	}
	ext := strings.ToLower(filepath.Ext(path))
	if !codeExts[ext] || ext == ".html" || ext == ".go" { // gofmt owns Go's layout
		return Msg{}
	}
	const strong = 0.8
	if r.IndentShare >= strong && r.Indent != "" {
		if got, n := indentOf(after); got != "" && n >= 5 && got != r.Indent {
			name := map[string]Msg{"tabs": M("conv.indent_tabs"), "2": M("conv.indent_2"), "4": M("conv.indent_4")}
			return M("conv.indent", "want", name[r.Indent], "share", int(r.IndentShare*100+0.5), "got", name[got])
		}
	}
	if jsLikeExts[ext] {
		added := addedLines(before, after)
		if r.QuotesShare >= strong && r.Quotes != "" {
			single, double := 0, 0
			for _, l := range added {
				if t := strings.TrimSpace(l); strings.HasPrefix(t, "import ") || strings.Contains(t, "require(") {
					if strings.Contains(t, "'") {
						single++
					} else if strings.Contains(t, `"`) {
						double++
					}
				}
			}
			if r.Quotes == "single" && double > single && double >= 2 || r.Quotes == "double" && single > double && single >= 2 {
				return map[string]Msg{"single": M("conv.quotes_single"), "double": M("conv.quotes_double")}[r.Quotes]
			}
		}
		if r.SemisShare >= strong && r.Semis != "" {
			with, without := 0, 0
			for _, l := range added {
				t := strings.TrimSpace(l)
				if t == "" || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "*") {
					continue
				}
				if strings.HasSuffix(t, ";") {
					with++
				} else if strings.HasSuffix(t, ")") || strings.HasSuffix(t, "'") || strings.HasSuffix(t, `"`) {
					without++
				}
			}
			total := with + without
			if total >= 6 && (r.Semis == "with" && without*100/total >= 70 || r.Semis == "without" && with*100/total >= 70) {
				return map[string]Msg{"with": M("conv.semis_with"), "without": M("conv.semis_without")}[r.Semis]
			}
		}
		if r.ModShare >= strong && r.Modules != "" && ext != ".ts" && ext != ".tsx" && ext != ".mjs" && ext != ".cjs" {
			esm := regexp.MustCompile(`(?m)^\s*(import\s.+from\s|import\s+['"]|export\s)`).MatchString(after)
			cjs := strings.Contains(after, "require(") || strings.Contains(after, "module.exports")
			if r.Modules == "cjs" && esm && !cjs || r.Modules == "esm" && cjs && !esm {
				return M("conv.modules", "system", map[string]string{"cjs": "CommonJS (require/module.exports)", "esm": "ESM (import/export)"}[r.Modules])
			}
		}
	}
	if before == "" && r.Naming != nil { // a new file's name
		if want, share := r.Naming[ext], r.NamingShare[ext]; want != "" && share >= strong && r.NamingN[ext] >= 5 {
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			base = strings.TrimSuffix(strings.TrimSuffix(base, ".test"), ".spec")
			if got := namingStyle(base); got != "" && got != want {
				return M("conv.naming", "ext", ext, "want", want, "file", filepath.Base(path), "got", got)
			}
		}
	}
	return Msg{}
}

func namingStyle(name string) string {
	switch {
	case knowledge.ReKebab.MatchString(name):
		return "kebab-case"
	case knowledge.ReSnake.MatchString(name):
		return "snake_case"
	case knowledge.ReCamel.MatchString(name):
		return "camelCase"
	case knowledge.RePascal.MatchString(name):
		return "PascalCase"
	}
	return ""
}

// addedLines is a cheap line diff: the lines of after that before does not have.
func addedLines(before, after string) []string {
	have := map[string]int{}
	for _, l := range strings.Split(before, "\n") {
		have[l]++
	}
	var out []string
	for _, l := range strings.Split(after, "\n") {
		if have[l] > 0 {
			have[l]--
			continue
		}
		out = append(out, l)
	}
	return out
}

// ---------------------------------------------------------------- obligations before the turn ends

// turnObligation returns what the agent still has to do before the turn may end (empty when nothing).
func (s *Session) turnObligation(ctx context.Context) Msg {
	s.mu.Lock()
	changed := append([]CheckpointFile{}, s.Checkpoints[s.Turn]...)
	asks := s.gate.obligationAsks
	var open []string
	for _, t := range s.Tasks { // the session's plan, whichever turn registered it
		if t.Status != "done" && t.Status != "blocked" {
			open = append(open, t.Text)
		}
	}
	s.mu.Unlock()
	if asks >= 6 { // the agent keeps refusing: the turn ends, and infuse closes what was left as blocked
		s.closeOpenTasks(M("plan.closed_note").Render(s.uiLang())) // shown only in the plan pane
		return Msg{}
	}
	if len(open) > 0 {
		return M("turn.open_tasks", "tasks", strings.Join(open, "; "))
	}
	if len(changed) == 0 {
		return Msg{}
	}
	// the project's checker on every file changed in the turn
	var problems []string
	for _, c := range changed {
		abs := filepath.Join(s.workRoot(), filepath.FromSlash(c.Path))
		if _, err := os.Stat(abs); err != nil {
			continue
		}
		if d := fileDiagnostics(s.Root, abs); d != "" {
			problems = append(problems, c.Path+":\n"+d)
		}
	}
	if len(problems) > 0 {
		return M("turn.checker", "problems", truncate(strings.Join(problems, "\n\n"), 4000))
	}
	if why := s.missingVerification(); !why.Empty() {
		return why
	}
	if why := s.missingReport(); !why.Empty() {
		return why
	}
	// the mandatory independent review, once per turn (its findings must be answered)
	if s.settings.RequireReview == nil || *s.settings.RequireReview {
		s.mu.Lock()
		reviewed := s.gate.reviewed
		s.gate.reviewed = true
		s.mu.Unlock()
		if !reviewed {
			out, err := s.RunSubagent(ctx, "review", M("turn.review_title").Render(s.uiLang()), s.reviewMaterial("")+"\nReview the changes above. End with a line exactly like this: VERDICT: APPROVE or VERDICT: FIX.")
			if err == nil && !regexp.MustCompile(`(?i)(VERDICT:\s*APPROVE|VEREDITO:\s*APROVAR)`).MatchString(out) {
				return M("turn.review", "findings", out)
			}
		}
	}
	return Msg{}
}

// closeOpenTasks marks every open task blocked with infuse's note: the plan never shows work as
// ongoing once the agent has stopped.
func (s *Session) closeOpenTasks(note string) {
	s.mu.Lock()
	tasks := append([]TaskItem{}, s.Tasks...)
	s.mu.Unlock()
	changed := false
	for i := range tasks {
		if tasks[i].Status != "done" && tasks[i].Status != "blocked" {
			tasks[i].Status, tasks[i].Note, changed = "blocked", note, true
		}
	}
	if changed {
		Journal(s.Root, JournalEntry{Kind: "gate", Detail: "plano encerrado com tarefas abertas"})
		s.setTasks(tasks)
	}
}

// hostOf returns a URL's host (for messages).
func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return u.Host
	}
	return raw
}
