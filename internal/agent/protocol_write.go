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
	if strings.Contains(rel, ".infuse/") || strings.HasSuffix(rel, ".md") && !strings.Contains(rel, "/src/") {
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
