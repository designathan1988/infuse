package agent

import (
	"encoding/json"
	"infuse/internal/knowledge"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Write-time policy: plan, docs-before-use, conventions, architecture, size.

func (s *Session) protocolCheckWrite(name string, input json.RawMessage) Msg {
	p, before, after, ok := editedContent(s.workRoot(), s.Cwd, name, input)
	if !ok {
		return Msg{}
	}
	rel := filepath.ToSlash(p)
	if strings.HasPrefix(filepath.ToSlash(relTo(s.workRoot(), p)), ".infuse/") || strings.HasSuffix(rel, ".md") && !strings.Contains(rel, "/src/") {
		return Msg{} // infuse's own files and prose documents are not code
	}
	// 0. the project's instructions: not edited, references not written, the declared measurements used
	if why := s.instructionWriteCheck(relTo(s.workRoot(), p), before); !why.Empty() {
		return why
	}
	// 1. a plan for this turn
	s.Mu.Lock()
	planned := s.gate.planned
	s.Mu.Unlock()
	if !planned {
		return M("protocol.no_plan")
	}
	// 2. the request's contract: the user confirmed what done means before code moves
	if why := s.contractGate(); !why.Empty() {
		return why
	}
	ext := strings.ToLower(filepath.Ext(p))
	if m := s.docsBeforeUse(p, before, after); !m.Empty() {
		return m
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
	// 5. the user's base: code lives where the chosen architecture says
	if why := s.baseViolation(relTo(s.workRoot(), p), codeExts[ext], after); !why.Empty() {
		return why
	}
	return s.sizeViolation(rel, ext, before, after)
}

// docsBeforeUse refuses a new library or host whose documentation was not consulted this turn.
func (s *Session) docsBeforeUse(p, before, after string) Msg {
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
	if len(missing) == 0 {
		return Msg{}
	}
	sort.Strings(missing)
	return M("protocol.docs_first", "uses", strings.Join(missing, ", "))
}

func (s *Session) sizeViolation(rel, ext, before, after string) Msg {
	limit := s.fileLineLimit()
	if limit <= 0 || !codeExts[ext] {
		return Msg{}
	}
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
	if m := indentViolation(r, after); !m.Empty() {
		return m
	}
	if jsLikeExts[ext] {
		if m := jsConventionViolation(r, ext, before, after); !m.Empty() {
			return m
		}
	}
	if before == "" {
		return namingViolation(r, path, ext)
	}
	return Msg{}
}

func indentViolation(r *knowledge.ConventionRules, after string) Msg {
	const strong = 0.8
	if r.IndentShare < strong || r.Indent == "" {
		return Msg{}
	}
	got, n := indentOf(after)
	if got == "" || n < 5 || got == r.Indent {
		return Msg{}
	}
	name := map[string]Msg{"tabs": M("conv.indent_tabs"), "2": M("conv.indent_2"), "4": M("conv.indent_4")}
	return M("conv.indent", "want", name[r.Indent], "share", int(r.IndentShare*100+0.5), "got", name[got])
}

func jsConventionViolation(r *knowledge.ConventionRules, ext, before, after string) Msg {
	const strong = 0.8
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
	if m := semicolonViolation(r, added); !m.Empty() {
		return m
	}
	if r.ModShare >= strong && r.Modules != "" && ext != ".ts" && ext != ".tsx" && ext != ".mjs" && ext != ".cjs" {
		esm := regexp.MustCompile(`(?m)^\s*(import\s.+from\s|import\s+['"]|export\s)`).MatchString(after)
		cjs := strings.Contains(after, "require(") || strings.Contains(after, "module.exports")
		if r.Modules == "cjs" && esm && !cjs || r.Modules == "esm" && cjs && !esm {
			return M("conv.modules", "system", map[string]string{"cjs": "CommonJS (require/module.exports)", "esm": "ESM (import/export)"}[r.Modules])
		}
	}
	return Msg{}
}

func semicolonViolation(r *knowledge.ConventionRules, added []string) Msg {
	const strong = 0.8
	if r.SemisShare < strong || r.Semis == "" {
		return Msg{}
	}
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
	return Msg{}
}

func namingViolation(r *knowledge.ConventionRules, path, ext string) Msg {
	const strong = 0.8
	if r.Naming == nil {
		return Msg{}
	}
	want, share := r.Naming[ext], r.NamingShare[ext]
	if want == "" || share < strong || r.NamingN[ext] < 5 {
		return Msg{}
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".test"), ".spec")
	if got := namingStyle(base); got != "" && got != want {
		return M("conv.naming", "ext", ext, "want", want, "file", filepath.Base(path), "got", got)
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
