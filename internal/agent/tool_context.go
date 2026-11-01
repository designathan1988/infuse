package agent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
			return "", errMsg(M("path.outside", "path", p))
		}
	}
	if !forWrite {
		// A read is judged against the real path too: a junction or symlink inside the project must
		// not turn a protected record into readable content.
		real := realPath(abs)
		if real != abs {
			if relReal, rerr := filepath.Rel(tc.Root, real); rerr == nil && !strings.HasPrefix(relReal, "..") && hiddenInternal(relReal) {
				return "", errMsg(M("path.internal", "path", toPosix(relReal)))
			}
			if tc.S != nil && tc.S.isProtected(real) {
				return "", errMsg(M("path.protected", "path", toPosix(rel)))
			}
		}
		if err == nil && !strings.HasPrefix(rel, "..") && hiddenInternal(rel) {
			return "", errMsg(M("path.internal", "path", toPosix(rel)))
		}
	}
	if forWrite && tc.S.isProtected(abs) {
		Journal(tc.Root, JournalEntry{Kind: "blocked", Path: toPosix(rel), Detail: "attempt to write to a protected path"})
		Lesson(tc.Root, "protected-path-write", "attempt to write to "+toPosix(rel))
		return "", errMsg(M("path.protected", "path", toPosix(rel)))
	}
	return abs, nil
}

// realPath resolves the deepest existing part of a path, so a link that does not exist yet is still
// read as the place it points to.
func realPath(abs string) string {
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	if r, err := filepath.EvalSymlinks(filepath.Dir(abs)); err == nil {
		return filepath.Join(r, filepath.Base(abs))
	}
	return abs
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
	if m := LoadMap(tc.Root); m != nil {
		if d := DependentsMsg(m, rel); !d.Empty() {
			lines = append(lines, tc.Say(d))
		}
	}
	for _, m := range ApplyWriteRulesMsgs(tc.Root, tc.Rules, []string{abs}) {
		lines = append(lines, tc.Say(m))
	}
	if tc.S.settings.Diagnostics == nil || *tc.S.settings.Diagnostics {
		if d := fileDiagnostics(tc.Root, abs); d != "" {
			lines = append(lines, tc.Say(M("write.diagnostics", "problems", d)))
		}
	}
	return lines
}
func init() { registerBuiltins() }
