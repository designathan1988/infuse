package filter

// The guard: protected paths (.infuse/oracle/** always, plus the user's list) are unreachable for the
// agent through EVERY tool, enforced in the executor, never by text. Write/Edit are refused at the path
// gate (resolvePath); shell commands pass through GuardCommand before anything runs or is transformed.
// Every path is compared in its canonical spelling and at its real place (canon.go).
//
// A command is read the way the shell will run it: split into simple commands (;, &&, ||, |, &, newline,
// subshells and groups), with the working directory followed across cd / pushd / popd, pipelines kept
// together, the payload of nested shells (bash -c, sh -c, eval, cmd /c, powershell -Command) and command
// substitutions guarded strictly, and symlinks resolved to their targets. Heredoc bodies are masked
// first, so document text never looks like a command. The OS sandbox is the wall for writes; the
// parser's job is the good message and the journal entry. Reads have no such wall (guard_read.go).
//   - heredoc into a protected file             → refused (before the transform could write it)
//   - heredoc outside the project or worktree   → refused: the transform writes on the host, uncontained
//   - redirection (>, >>, >|, <>, &>, >&) into one → neutralized to /dev/null, the model is told
//     (inside a nested shell: refused — the payload cannot be rewritten safely)
//   - a writing/destroying command naming one    → refused (rm, mv, cp, tee, sed -i, find -delete/-exec,
//     xargs with a writer in a pipeline that names one, git rm/clean/stash/checkout/restore, del,
//     Remove-Item…), and so is one that removes or overwrites a folder holding one (guard_tree.go)
// Every attempt goes to the journal as "blocked" and to the lessons.

import (
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/lang"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	oracleRel    = ".infuse/oracle"
	evidenceRel  = ".infuse/evidence" // the executor's verdicts and evidence: written only by the executor
	worktreesRel = ".infuse/worktrees"
)

type Guard struct {
	Root      string
	Work      string   // the session's worktree ("" without one): writes stay in it, policy matches in it
	Protected []string // globs relative to root
	roots     []string // Work then Root, as spelled and at their real place
}

// infuse's own records (other sessions' transcripts, the journal, snapshots, checkpoints) are not the
// project: the agent neither reads nor writes them, by any tool. The project settings, hooks and MCP
// configuration are here because a single write to them could disarm the filter or self-approve a
// command; they are the user's, not the agent's. So are the rules (they rewrite commands and turn the
// transform off), the launch configurations, and the commands and skills (text that becomes
// instructions): the panel writes those for the user, and .infuse/skills stays readable.
var internalRels = []string{
	".infuse/sessions", ".infuse/checkpoints", ".infuse/tree", ".infuse/journal.jsonl",
	".infuse/watcher.json", ".infuse/architecture.json", ".infuse/conventions.json",
	".infuse/project-rules.json", ".infuse/settings.json", ".infuse/settings.local.json", ".infuse/hooks.json", ".infuse/base.json",
	".infuse/plugins", ".infuse/turnlog",
	".infuse/rules.json", ".infuse/launch.json", ".infuse/commands", ".infuse/skills",
	".infuse/policy.json",
}

// externalRels are files shared with other agents that the agent must never rewrite, although they
// live outside .infuse: the project's MCP servers and Claude Code's own settings.
var externalRels = []string{".mcp.json", ".claude/settings.json", ".claude/settings.local.json"}

// gitControlRels are the parts of a repository that run code on the host: a hook, or a config entry
// such as core.hooksPath or core.fsmonitor, executes the next time anyone uses git.
var gitControlRels = []string{".git/config", ".git/hooks"}

// InfuseRecords lists the project-relative paths whose writes belong to infuse alone: its records
// plus the oracle and the evidence. The sandbox denies them at the OS level with the same list.
func InfuseRecords() []string {
	out := append(append(append([]string{}, internalRels...), externalRels...), gitControlRels...)
	return append(out, oracleRel, evidenceRel)
}

// ContainedRecords is what the OS containment keeps out of a shell's reach: infuse's records plus the
// user's protected paths (literal paths or globs).
func ContainedRecords(userProtected []string) []string {
	return append(InfuseRecords(), userProtected...)
}

// GitControl reports whether a path is a git hook or a repository config, which is refused with its
// own explanation: the user reads why, not only that.
func GitControl(p string) bool { return containsOnly(p, gitControlRels) }

// agentVisibleInternal lists what of .infuse/ the agent may read: the oracle's criteria (SPEC §5: the
// agent sees them and cannot reach the executor), pinned docs, the conventions, the project map and
// its own captures.
var agentVisibleInternal = []string{".infuse/oracle/", ".infuse/docs/", ".infuse/conventions.md", ".infuse/map/MAP.md", ".infuse/map/dirs/", ".infuse/shots/", ".infuse/recordings/", ".infuse/skills/", ".infuse/architecture.json", ".infuse/project-rules.json"}

// hiddenInternal reports whether rel (relative to the project) is infuse's own file the agent may not read.
func HiddenInternal(rel string) bool {
	rel = foldPath(strings.TrimPrefix(core.ToPosix(rel), "./"))
	if rel != ".infuse" && !strings.HasPrefix(rel, ".infuse/") {
		return false
	}
	for _, ok := range agentVisibleInternal {
		ok = foldPath(ok)
		if rel == strings.TrimSuffix(ok, "/") || strings.HasPrefix(rel, ok) {
			return false
		}
	}
	return true
}

// foldPath lowers a path where the filesystem ignores case, so a spelling like .INFUSE cannot walk
// around a protection written in lower case.
func foldPath(p string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

// containsRel reports whether an absolute path contains one of the protected relative paths at any
// depth, so the same record is protected in a worktree or reached from outside the project root.
func containsRel(abs string, rels []string) bool {
	protected := append(append(append([]string{}, rels...), externalRels...), gitControlRels...)
	return containsOnly(abs, append(protected, oracleRel, evidenceRel))
}

func containsOnly(abs string, rels []string) bool {
	a := foldPath("/" + strings.Trim(filepath.ToSlash(abs), "/"))
	for _, rel := range rels {
		r := foldPath(rel)
		if strings.HasSuffix(a, "/"+r) || strings.Contains(a, "/"+r+"/") {
			return true
		}
	}
	return false
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
	for _, r := range externalRels {
		base = append(base, r)
	}
	for _, r := range gitControlRels {
		base = append(base, r+"/**", r)
	}
	base = append(base, p.InstructionFiles...) // the project's instructions: followed, not edited
	for _, ref := range p.References {
		base = append(base, ref+"/**", ref)
	}
	return &Guard{Root: root, Protected: append(base, p.Protected...), roots: guardRoots("", root)}
}

// In binds the guard to a session's worktree: the project's policy applies to the worktree's own files,
// and a heredoc stays inside it, as Write does.
func (g *Guard) In(worktree string) *Guard {
	g.Work = worktree
	g.roots = guardRoots(worktree, g.Root)
	return g
}

func guardRoots(work, root string) []string {
	var out []string
	for _, r := range []string{work, root} {
		if r != "" {
			out = append(out, r, RealPath(r))
		}
	}
	return out
}

// Protects reports whether p (absolute, or relative to cwd) is agent-proof, in any spelling. A symlink
// counts as its target and a wildcard as what it expands to.
func (g *Guard) Protects(cwd, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || p == "/dev/null" || strings.EqualFold(p, "NUL") || strings.HasPrefix(p, "-") {
		return "", false
	}
	for _, abs := range pathSpellings(CanonPath(cwd, p)) {
		if rel, ok := g.protectsAbs(abs); ok {
			return rel, true
		}
	}
	return "", false
}

func (g *Guard) protectsAbs(abs string) (string, bool) {
	// The oracle, the evidence and infuse's own records are protected wherever they are: the
	// project's, a worktree's, or reached from outside the root with ../
	if containsRel(abs, internalRels) {
		rel := core.RelTo(g.Root, abs)
		if rel == "" || strings.HasPrefix(rel, "..") {
			rel = filepath.ToSlash(abs)
		}
		return rel, true
	}
	for _, rel := range g.policyRels(abs) {
		for _, pat := range g.Protected {
			if matchGlob(pat, rel) || foldPath(rel) == foldPath(strings.TrimSuffix(pat, "/**")) {
				return rel, true
			}
		}
	}
	return "", false
}

// policyRels is abs relative to every root that holds it. A worktree is a copy of the project, so the
// user's globs, the references, the instructions and the memory match inside it as they do at the
// root; a worktree in the default folder is recognized without being told.
func (g *Guard) policyRels(abs string) []string {
	var out []string
	for _, root := range g.roots {
		if !core.Within(root, abs) {
			continue
		}
		rel := core.RelTo(root, abs)
		if rel == "" {
			continue
		}
		out = append(out, rel)
		if tail := worktreeTail(rel); tail != "" {
			out = append(out, tail)
		}
	}
	return out
}

// worktreeTail cuts .infuse/worktrees/<slug>/ from a project-relative path ("" when it is not there).
func worktreeTail(rel string) string {
	parts := strings.SplitN(rel, "/", 4)
	if len(parts) == 4 && foldPath(parts[0]+"/"+parts[1]) == worktreesRel {
		return parts[3]
	}
	return ""
}

// matchGlob matches a protected glob against a relative path, ignoring case where the OS does.
func matchGlob(pat, rel string) bool {
	if runtime.GOOS == "windows" {
		pat, rel = strings.ToLower(pat), strings.ToLower(rel)
	}
	return lang.GlobToRegex(pat).MatchString(rel)
}

// block records a refused write to a protected path; the error carries the message (msgOf).
func (g *Guard) block(rel string, detail core.Msg) error {
	journal.Journal(g.Root, journal.JournalEntry{Kind: "blocked", Path: rel, Detail: detail.String()})
	journal.Lesson(g.Root, "protected-path-write", detail.String()+": "+rel)
	if GitControl(rel) {
		return core.ErrMsg(core.M("guard.git_control", "file", rel, "detail", detail))
	}
	return core.ErrMsg(core.M("guard.protected", "file", rel, "detail", detail))
}

// ReRedir: output redirections and the read-write open `<>` (group 3 is the target).
