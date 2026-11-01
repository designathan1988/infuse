package agent

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
	"encoding/json"
	"path/filepath"
	"regexp"
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
	s.Mu.Lock()
	s.gate.researched = append(s.gate.researched, text)
	s.Mu.Unlock()
}

// researchedFor reports whether name (a package or a host) was consulted in this turn or is pinned.
func (s *Session) researchedFor(name string) bool {
	n := strings.ToLower(name)
	alt := strings.ReplaceAll(n, "-", " ")
	s.Mu.Lock()
	seen := append([]string{}, s.gate.researched...)
	s.Mu.Unlock()
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
	localHosts  = map[string]bool{"localhost": true, "127.0.0.1": true, "0.0.0.0": true}
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
