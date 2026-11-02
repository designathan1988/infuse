package agent

import (
	"context"
	"encoding/json"
	"infuse/internal/filter"
	"os"
	"path/filepath"
)

// The tool contract, its context, the registry and the shared path/output helpers.

type ToolContext struct {
	CallID string // the tool call being served (messages are recorded with it)
	S      *Session
	Root   string
	Cwd    string
	Rules  *RuleSet
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

// clip keeps the head and the tail of a long output; the cut is said where it happened.
func (tc *ToolContext) clip(s string) string {
	if len(s) <= maxToolOutput {
		return s
	}
	return s[:maxToolOutput/2] + "\n\n" + tc.Say(M("tool.omitted", "count", len(s)-maxToolOutput)) + "\n\n" + s[len(s)-maxToolOutput/2:]
}

// resolvePath turns a tool path into its canonical absolute path. A write must land inside the work
// root and off every protected path; a read may leave the project (the permission gate asks) but never
// reaches a hidden record. Both are judged in every spelling and at the real place of the path.
func (tc *ToolContext) resolvePath(p string, forWrite bool) (string, error) {
	abs := filter.CanonPath(tc.Cwd, p)
	var err error
	if forWrite {
		err = tc.gateWrite(p, abs)
	} else {
		err = tc.gateRead(abs)
	}
	if err != nil {
		return "", err
	}
	return abs, nil
}

func (tc *ToolContext) gateWrite(p, abs string) error {
	rel := toPosix(relTo(tc.Root, abs))
	if !within(realPath(tc.Root), realPath(abs)) {
		tc.blocked(rel, "attempt to write outside the project")
		return errMsg(M("path.outside", "path", p))
	}
	if !tc.S.isProtected(abs) {
		return nil
	}
	tc.blocked(rel, "attempt to write to a protected path")
	Lesson(tc.journalRoot(), "protected-path-write", "attempt to write to "+rel)
	if filter.GitControl(rel) {
		return errMsg(M("path.git_control", "path", rel))
	}
	return errMsg(M("path.protected", "path", rel))
}

func (tc *ToolContext) gateRead(abs string) error {
	if rel, hidden := tc.hiddenRead(abs); hidden {
		tc.blocked(rel, "attempt to read a hidden record")
		return errMsg(M("path.internal", "path", rel))
	}
	// a junction or symlink inside the project must not turn a protected file into readable content
	if real := realPath(abs); real != abs && tc.S != nil && tc.S.isProtected(real) {
		rel := toPosix(relTo(tc.Root, abs))
		tc.blocked(rel, "attempt to read a protected path through a link")
		return errMsg(M("path.protected", "path", rel))
	}
	return nil
}

// blocked journals a refused attempt where the project's records live, not in a session's worktree.
func (tc *ToolContext) blocked(rel, detail string) {
	Journal(tc.journalRoot(), JournalEntry{Kind: "blocked", Path: rel, Detail: detail})
}

func (tc *ToolContext) journalRoot() string {
	if tc.S != nil {
		return tc.S.Root
	}
	return tc.Root
}

// readRoots are the folders whose own files are not records, whatever their path says.
func (tc *ToolContext) readRoots() []string {
	roots := []string{tc.Root, realPath(tc.Root)}
	if tc.S != nil {
		roots = append(roots, tc.S.Root, realPath(tc.S.Root))
	}
	return roots
}

// hiddenRead reports whether abs is a hidden record, as spelled or at its real place, from any starting
// point: a worktree reaches the project's records with ../../../ just as well.
func (tc *ToolContext) hiddenRead(abs string) (string, bool) {
	roots := tc.readRoots()
	if rel, hidden := filter.HiddenAt(abs, roots...); hidden {
		return rel, true
	}
	return filter.HiddenAt(realPath(abs), roots...)
}

// walkBase is the folder a Grep or a Glob starts from: the given one, or the work root.
func (tc *ToolContext) walkBase(p string) (string, error) {
	if p == "" {
		return tc.Root, nil
	}
	return tc.resolvePath(p, false)
}

// walkFilter is hiddenRead for the tools that walk a tree: the name decides, and only a link costs a
// look at the disk.
func (tc *ToolContext) walkFilter() func(p string, d os.DirEntry) bool {
	roots := tc.readRoots()
	return func(p string, d os.DirEntry) bool {
		if d.Type()&os.ModeSymlink != 0 {
			_, hidden := tc.hiddenRead(p)
			return hidden
		}
		_, hidden := filter.HiddenAt(p, roots...)
		return hidden
	}
}

// realPath resolves the deepest existing part of a path, so a link that does not exist yet is still
// judged as the place it points to.
func realPath(abs string) string { return filter.RealPath(abs) }

// guard is the session's guard, rooted at the real project (from a worktree, ../../ reaches the
// project's own records and settings) and bound to its worktree (the project's policy applies there).
func (s *Session) guard() *filter.Guard {
	return newGuard(s.Root, s.settings.ProtectedPaths).In(s.Worktree)
}

// refuseProtectedWrite runs before every other check of a write tool: a gate that stops the call first
// (the diagnosis, the plan) must not hide the attempt from the journal.
func (s *Session) refuseProtectedWrite(tool Tool, b Block) (string, bool) {
	p := toolPath(b.Input)
	if tool.Category() != "write" || p == "" {
		return "", false
	}
	s.Mu.Lock()
	tc := &ToolContext{CallID: b.ID, S: s, Root: s.workRoot(), Cwd: s.Cwd}
	s.Mu.Unlock()
	if _, err := tc.resolvePath(p, true); err != nil {
		return tc.SayErr(err), true
	}
	return "", false
}

func (tc *ToolContext) afterWrite(abs string) []string {
	rel := relTo(tc.Root, abs)
	st, _ := os.Stat(abs)
	size := 0
	if st != nil {
		size = int(st.Size())
	}
	Journal(tc.Root, JournalEntry{Kind: "write", Path: rel, Bytes: size, Detail: "agent"})
	tc.S.scheduleMapRefresh()
	var lines []string
	if inst := tc.S.localInstructions(filepath.Dir(abs)); inst != "" {
		lines = append(lines, tc.Say(M("read.localInstructions"))+"\n"+inst)
	}
	if m := LoadMap(tc.Root); m != nil {
		if d := DependentsMsg(m, rel); !d.Empty() {
			lines = append(lines, tc.Say(d))
		}
	}
	for _, m := range ApplyWriteRulesMsgs(tc.Root, tc.Rules, []string{abs}) {
		lines = append(lines, tc.Say(m))
	}
	if tc.S.settings.Diagnostics == nil || *tc.S.settings.Diagnostics {
		if d, done := diagnosticsWithin(tc.Root, abs, diagnosticsBudget); !done {
			lines = append(lines, tc.Say(M("write.diagnostics_later")))
		} else if d != "" {
			lines = append(lines, tc.Say(M("write.diagnostics", "problems", d)))
		}
	}
	return lines
}
func init() { registerBuiltins() }
