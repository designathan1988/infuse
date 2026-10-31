package main

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
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// instructionNames are the files agents read as the project's instructions.
var instructionNames = []string{"CLAUDE.md", "AGENTS.md", ".claude/CLAUDE.md", "GEMINI.md", ".github/copilot-instructions.md", ".cursorrules", ".infuse/instructions.md"}

type projectRules struct {
	Files       []string // instruction files found (relative)
	Required    []string // documents they point to (relative), to be read in full
	Reference   []string // paths they forbid copying or writing (relative, may be outside the project as ../x)
	Commands    []string // commands they declare for verifying the work
	AfterChange []string // the declared commands to run after changes
	Missing     []string // instruction files deleted from the working tree (read from git)
	text        string   // all instruction text, @imports resolved
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
	rules projectRules
}

// loadProjectRules reads the instruction files (from git when deleted) and extracts their locks.
func loadProjectRules(root string) projectRules {
	stamp := ""
	for _, n := range instructionNames {
		if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(n))); err == nil {
			stamp += n + fi.ModTime().String()
		}
	}
	if c, ok := rulesCache.Load(root); ok && c.(cachedRules).stamp == stamp && time.Since(c.(cachedRules).at) < 15*time.Second {
		return c.(cachedRules).rules
	}
	var r projectRules
	var texts []string
	for _, n := range instructionNames {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			if t := gitHeadFile(root, n); t != "" { // deleted in the working tree: still the project's rules
				r.Missing = append(r.Missing, n)
				b = []byte(t)
			} else {
				continue
			}
		}
		r.Files = append(r.Files, n)
		texts = append(texts, resolveInstructionImports(root, filepath.Dir(filepath.Join(root, filepath.FromSlash(n))), string(b), 0))
	}
	r.text = strings.Join(texts, "\n\n")
	clean := reURL.ReplaceAllString(r.text, " ")
	seenReq := map[string]bool{}
	addRequired := func(p string) {
		p = strings.TrimSuffix(strings.TrimPrefix(filepath.ToSlash(p), "./"), "/")
		fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil || fi.IsDir() || !docExts[strings.ToLower(filepath.Ext(p))] || fi.Size() >= 400<<10 || seenReq[p] || contains(r.Files, p) || len(r.Required) >= 24 {
			return
		}
		seenReq[p] = true
		r.Required = append(r.Required, p)
	}
	// documents the instructions point to: paths are paths in any language
	for _, m := range rePathToken.FindAllStringSubmatch(clean, -1) {
		if !strings.Contains(m[1], "*") && !strings.HasPrefix(m[1], "node_modules") {
			addRequired(m[1])
		}
	}
	// what the words mean (references, documents to follow, commands to verify): compiled, quote-checked
	if c := loadCompiledRules(root); c != nil {
		for _, cr := range c.Rules {
			if cr.Disabled {
				continue
			}
			switch cr.Kind {
			case "read":
				addRequired(cr.Path)
			case "reference":
				if p := strings.Trim(strings.TrimPrefix(filepath.ToSlash(cr.Path), "./"), "/"); !contains(r.Reference, p) {
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
	sort.Strings(r.Required)
	rulesCache.Store(root, cachedRules{stamp, time.Now(), r})
	return r
}

// resolveImports inlines Claude Code style "@path" lines (a file imported by the instructions).
func resolveInstructionImports(root, dir, text string, depth int) string {
	if depth > 3 {
		return text
	}
	return reImportLine.ReplaceAllStringFunc(text, func(line string) string {
		p := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "@"))
		abs := filepath.Join(dir, filepath.FromSlash(p))
		if !within(root, abs) {
			return line
		}
		b, err := os.ReadFile(abs)
		if err != nil || len(b) > 300<<10 {
			return line
		}
		return line + "\n<!-- imported from " + p + " -->\n" + resolveInstructionImports(root, filepath.Dir(abs), string(b), depth+1)
	})
}

// gitHeadFile is a file's committed content ("" when not tracked or no git).
func gitHeadFile(root, rel string) string {
	if !exists(filepath.Join(root, ".git")) {
		return ""
	}
	cmd := exec.Command("git", "show", "HEAD:"+rel)
	cmd.Dir = root
	noConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// isInstructionFile reports whether rel is one of the project's instruction files.
func isInstructionFile(rel string) bool {
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, n := range instructionNames {
		if strings.EqualFold(rel, n) {
			return true
		}
	}
	return false
}

// underReference reports the forbidden reference rel lies in ("" when none).
func underReference(r projectRules, rel string) string {
	rel = filepath.ToSlash(rel)
	for _, ref := range r.Reference {
		if rel == ref || strings.HasPrefix(rel, ref+"/") {
			return ref
		}
	}
	return ""
}
