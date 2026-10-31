package filter

// The guard: protected paths (.infuse/oracle/** always, plus the user's list) are unreachable for the
// agent through EVERY tool, enforced in the executor, never by text. Write/Edit are refused at the path
// gate (resolvePath); shell commands pass through GuardCommand before anything runs or is transformed.
//
// A command is read the way the shell will run it: split into simple commands (;, &&, ||, |, &, newline),
// with the working directory followed across cd / pushd / popd, pipelines kept together, the payload of
// nested shells (bash -c, sh -c, eval, cmd /c, powershell -Command) guarded strictly, and symlinks resolved
// to their targets. Heredoc bodies are masked first, so document text never looks like a command.
//   - heredoc into a protected file             → refused (before the transform could write it)
//   - redirection (>, >>, >|, <>) into one       → neutralized to /dev/null, the model is told
//     (inside a nested shell: refused — the payload cannot be rewritten safely)
//   - a writing/destroying command naming one    → refused (rm, mv, cp, tee, sed -i, find -delete/-exec,
//     xargs with a writer in a pipeline that names one, git rm/clean/stash/checkout/restore, del,
//     Remove-Item…)
// Every attempt goes to the journal as "blocked" and to the lessons.

import (
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"path/filepath"
	"strings"
)

const (
	oracleRel   = ".infuse/oracle"
	evidenceRel = ".infuse/evidence" // the executor's verdicts and evidence: written only by the executor
)

type Guard struct {
	Root      string
	Protected []string // globs relative to root
}

// infuse's own records (other sessions' transcripts, the journal, snapshots, checkpoints) are not the
// project: the agent neither reads nor writes them, by any tool.
var internalRels = []string{".infuse/sessions", ".infuse/checkpoints", ".infuse/tree", ".infuse/journal.jsonl", ".infuse/watcher.json", ".infuse/architecture.json", ".infuse/conventions.json", ".infuse/project-rules.json"}

// agentVisibleInternal lists what of .infuse/ the agent may read: pinned docs, the conventions, the
// project map and its own captures.
var agentVisibleInternal = []string{".infuse/docs/", ".infuse/conventions.md", ".infuse/map/MAP.md", ".infuse/shots/", ".infuse/recordings/", ".infuse/skills/", ".infuse/architecture.json", ".infuse/project-rules.json"}

// hiddenInternal reports whether rel (relative to the project) is infuse's own file the agent may not read.
func HiddenInternal(rel string) bool {
	rel = strings.TrimPrefix(core.ToPosix(rel), "./")
	if rel != ".infuse" && !strings.HasPrefix(rel, ".infuse/") {
		return false
	}
	for _, ok := range agentVisibleInternal {
		if rel == strings.TrimSuffix(ok, "/") || strings.HasPrefix(rel, ok) {
			return false
		}
	}
	return true
}

// Policy is the project-specific part of the protection, supplied by the caller so the filter never
// depends on the knowledge layer.
type Policy struct {
	Protected        []string // the user's protected globs
	References       []string // paths the project's instructions call read-only
	InstructionFiles []string // the instruction files themselves
	Memory           []string // the project's memory paths
}

func NewGuard(root string, p Policy) *Guard {
	base := []string{oracleRel + "/**", oracleRel, evidenceRel + "/**", evidenceRel}
	for _, m := range p.Memory {
		base = append(base, m+"/**", m)
	}
	for _, r := range internalRels {
		base = append(base, r+"/**", r)
	}
	base = append(base, p.InstructionFiles...) // the project's instructions: followed, not edited
	for _, ref := range p.References {
		base = append(base, ref+"/**", ref)
	}
	return &Guard{Root: root, Protected: append(base, p.Protected...)}
}

// protects reports whether p (absolute, or relative to cwd) is agent-proof. A symlink counts as its target.
func (g *Guard) Protects(cwd, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || p == "/dev/null" || strings.EqualFold(p, "NUL") || strings.HasPrefix(p, "-") {
		return "", false
	}
	p = core.FromMsysPath(strings.ReplaceAll(p, `\`, "/"))
	abs := p
	if !filepath.IsAbs(p) {
		abs = filepath.Join(cwd, p)
	}
	abs = filepath.Clean(abs)
	if rel, ok := g.protectsAbs(abs); ok {
		return rel, true
	}
	if real := resolveSymlinks(abs); real != abs {
		if rel, ok := g.protectsAbs(real); ok {
			return rel, true
		}
	}
	return "", false
}

func (g *Guard) protectsAbs(abs string) (string, bool) {
	rel := core.RelTo(g.Root, abs)
	// the oracle is protected wherever it is: the project's, a worktree's, or reached from outside the root
	if slash := "/" + strings.Trim(filepath.ToSlash(abs), "/"); strings.Contains(slash+"/", "/"+oracleRel+"/") || strings.Contains(slash+"/", "/"+evidenceRel+"/") {
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = filepath.ToSlash(abs)
		}
		return rel, true
	}
	if rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}
	for _, pat := range g.Protected {
		if lang.GlobToRegex(pat).MatchString(rel) || rel == strings.TrimSuffix(pat, "/**") {
			return rel, true
		}
	}
	return "", false
}

// resolveSymlinks resolves a path that may not exist yet (its deepest existing parent is resolved).
func resolveSymlinks(abs string) string {
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(r, filepath.Base(abs))
	}
	return abs
}

// block records a refused write to a protected path; the error carries the message (msgOf).
func (g *Guard) block(rel string, detail core.Msg) error {
	journal.Journal(g.Root, journal.JournalEntry{Kind: "blocked", Path: rel, Detail: detail.String()})
	journal.Lesson(g.Root, "protected-path-write", detail.String()+": "+rel)
	return core.ErrMsg(core.M("guard.protected", "file", rel, "detail", detail))
}

// ReRedir: output redirections and the read-write open `<>` (group 3 is the target).
