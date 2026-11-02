package knowledge

// The project's instructions, enforced — not only shown. An agent can have CLAUDE.md in its prompt and
// still act against it: not open the documents it points to, copy the code it calls a forbidden
// reference, invent its own measurements instead of the declared ones, or delete the file. infuse reads
// the instruction files and turns what is mechanical in them into locks:
//   required reading — every document they point to is read in full (line coverage) before the first
//                      change or command of the session;
//   forbidden references — paths they call read-only / never copy (compiled with a verified quote,
//                      instructions_compile.go) are not written to, and code copied from them is refused;
//   declared commands — the project's own checks are named to the agent, and those it orders after
//                      changes must have run, successfully, before a turn that changed code ends;
//   the files themselves — the agent never edits or deletes them; one missing from the working tree is
//                      read from git.

import (
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// InstructionNames are the files agents read as the project's instructions.
var InstructionNames = []string{"CLAUDE.md", "AGENTS.md", ".claude/CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md", ".cursorrules", ".infuse/instructions.md"}

type ProjectRules struct {
	Files       []string // instruction files found (relative)
	Required    []string // documents they point to (relative), to be read in full
	Reference   []string // paths they forbid copying or writing (relative, may be outside the project as ../x)
	Commands    []string // commands they declare for verifying the work
	AfterChange []string // the declared commands to run after changes
	Missing     []string // instruction files deleted from the working tree (read from git)
	Stale       bool     // the compiled rules are from an older instruction text (the latest compilation failed)
	Text        string   // all instruction text, @imports resolved
}

var (
	reURL        = regexp.MustCompile(`https?://\S+`)
	rePathToken  = regexp.MustCompile("(?:^|[\\s(\\[`\"'@])((?:\\.{1,2}/)?[\\w.-]+(?:/[\\w.*-]+)+/?|[\\w.-]+\\.(?:md|json|ya?ml|toml|txt))")
	reImportLine = regexp.MustCompile(`(?m)^\s*@([\w./-]+)\s*$`)
	docExts      = map[string]bool{".md": true, ".json": true, ".yml": true, ".yaml": true, ".toml": true, ".txt": true}
)

var rulesCache sync.Map // root → cachedRules

type cachedRules struct {
	stamp string
	at    time.Time
	rules ProjectRules
}

// InvalidateRules drops the cached instruction snapshot for root.
func InvalidateRules(root string) { rulesCache.Delete(root) }

// LoadProjectRules reads the instruction files (from git when deleted) and extracts their locks.
func LoadProjectRules(root string) ProjectRules {
	stamp := rulesStamp(root)
	if c, ok := rulesCache.Load(root); ok && c.(cachedRules).stamp == stamp && time.Since(c.(cachedRules).at) < 15*time.Second {
		return c.(cachedRules).rules
	}
	r, texts := readInstructionFiles(root)
	r.Text = strings.Join(texts, "\n\n")
	clean := reURL.ReplaceAllString(r.Text, " ")
	seenReq := map[string]bool{}
	addRequired := func(p string) { r.addRequired(root, p, seenReq) }
	requiredFromText(clean, addRequired)
	applyCompiledRules(root, &r, addRequired)
	sort.Strings(r.Required)
	rulesCache.Store(root, cachedRules{stamp, time.Now(), r})
	return r
}

// rulesStamp fingerprints the instruction files' names and modification times.
func rulesStamp(root string) string {
	stamp := ""
	for _, n := range InstructionNames {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(n))); err == nil {
			stamp += n + fi.ModTime().String()
		}
	}
	return stamp
}

// readInstructionFiles reads each instruction file (from git when deleted) and resolves its imports.
func readInstructionFiles(root string) (ProjectRules, []string) {
	var r ProjectRules
	var texts []string
	for _, n := range InstructionNames {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			if t := GitHeadFile(root, n); t != "" { // deleted in the working tree: still the project's rules
				r.Missing = append(r.Missing, n)
				b = []byte(t)
			} else {
				continue
			}
		}
		r.Files = append(r.Files, n)
		texts = append(texts, ResolveInstructionImports(root, filepath.Dir(filepath.Join(root, filepath.FromSlash(n))), string(b), 0))
	}
	return r, texts
}

// addRequired records p as a document to read in full, when it is an existing, small document.
func (r *ProjectRules) addRequired(root, p string, seen map[string]bool) {
	p = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(p), "./"), "/")
	if p == ".infuse" || strings.HasPrefix(p, ".infuse/") || filepath.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") {
		return // infuse's own records and paths outside the project are not readable project documents
	}
	fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
	if err != nil || fi.IsDir() || !docExts[strings.ToLower(filepath.Ext(p))] || fi.Size() >= 400<<10 || seen[p] || core.Contains(r.Files, p) || len(r.Required) >= 24 {
		return
	}
	// A binary document cannot be read by the agent; requiring it would make every action impossible.
	if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p))); err != nil || !utf8.Valid(b) {
		return
	}
	seen[p] = true
	r.Required = append(r.Required, p)
}

// requiredFromText adds the documents the instructions point to: paths are paths in any language.
func requiredFromText(clean string, addRequired func(string)) {
	for _, m := range rePathToken.FindAllStringSubmatch(clean, -1) {
		if !strings.Contains(m[1], "*") && !strings.HasPrefix(m[1], "node_modules") {
			addRequired(m[1])
		}
	}
}

// applyCompiledRules adds what the words mean: references, documents to follow, commands to verify
// (compiled and quote-checked).
func applyCompiledRules(root string, r *ProjectRules, addRequired func(string)) {
	c := LoadCompiledRules(root)
	if c == nil {
		return
	}
	r.Stale = c.Stale
	for _, cr := range c.Rules {
		if cr.Disabled {
			continue
		}
		switch cr.Kind {
		case "read":
			addRequired(cr.Path)
		case "reference":
			if p := strings.Trim(strings.TrimPrefix(filepath.ToSlash(cr.Path), "./"), "/"); !core.Contains(r.Reference, p) {
				r.Reference = append(r.Reference, p)
			}
		case "verify":
			r.Commands = append(r.Commands, cr.Command)
			if cr.When == "after-change" {
				r.AfterChange = append(r.AfterChange, cr.Command)
			}
		}
	}
}

// resolveImports inlines Claude Code style "@path" lines (a file imported by the instructions).
func ResolveInstructionImports(root, dir, text string, depth int) string {
	if depth > 3 {
		return text
	}
	return reImportLine.ReplaceAllStringFunc(text, func(line string) string {
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "@"))
		abs := filepath.Join(dir, filepath.FromSlash(p))
		if !core.Within(root, abs) {
			return line
		}
		b, err := os.ReadFile(abs)
		if err != nil || len(b) > 300<<10 {
			return line
		}
		return line + "\n<!-- imported from " + p + " -->\n" + ResolveInstructionImports(root, filepath.Dir(abs), string(b), depth+1)
	})
}

// GitHeadFile is a file's committed content ("" when not tracked or no git).
func GitHeadFile(root, rel string) string {
	if !core.Exists(filepath.Join(root, ".git")) {
		return ""
	}
	cmd := exec.Command("git", "show", "HEAD:"+rel)
	cmd.Dir = root
	process.NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// IsInstructionFile reports whether rel is one of the project's instruction files.
func IsInstructionFile(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, n := range InstructionNames {
		if strings.EqualFold(rel, n) {
			return true
		}
	}
	return false
}

// UnderReference reports the forbidden reference rel lies in ("" when none).
func UnderReference(r ProjectRules, rel string) string {
	rel = filepath.ToSlash(rel)
	for _, ref := range r.Reference {
		if rel == ref || strings.HasPrefix(rel, ref+"/") {
			return ref
		}
	}
	return ""
}
