package knowledge

// Convention learner: reads a sample of the project's source files and states its conventions
// (indentation, quotes, semicolons, module system, file naming, test layout, line endings, comment
// language) as short rules in .infuse/conventions.md. Counting, not guessing: every rule comes with the
// share of files it was seen in. The user can edit the file; the agent reads it every turn.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ConventionRules are the enforceable numeric rules learned from the project's code.
type ConventionRules struct {
	Indent      string             `json:"indent"` // tabs | 2 | 4
	IndentShare float64            `json:"indentShare"`
	Quotes      string             `json:"quotes"` // single | double
	QuotesShare float64            `json:"quotesShare"`
	Semis       string             `json:"semis"` // with | without
	SemisShare  float64            `json:"semisShare"`
	Modules     string             `json:"modules"` // esm | cjs
	ModShare    float64            `json:"modulesShare"`
	Naming      map[string]string  `json:"naming"` // ext → kebab-case | snake_case | camelCase | PascalCase
	NamingShare map[string]float64 `json:"namingShare"`
	NamingN     map[string]int     `json:"namingCount"`
	Files       int                `json:"files"`
}

func ConventionRulesFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "conventions.json")
}

func LoadConventionRules(root string) *ConventionRules {
	var r ConventionRules
	if core.ReadJSON(ConventionRulesFile(root), &r) != nil || r.Files == 0 {
		ConventionsPrompt(root) // learn them when missing
		if core.ReadJSON(ConventionRulesFile(root), &r) != nil || r.Files == 0 {
			return nil
		}
	}
	return &r
}

func ConventionsFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "conventions.md")
}

var (
	ReKebab  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)+$`)
	ReSnake  = regexp.MustCompile(`^[a-z0-9]+(_[a-z0-9]+)+$`)
	ReCamel  = regexp.MustCompile(`^[a-z][a-z0-9]*([A-Z][a-z0-9]*)+$`)
	RePascal = regexp.MustCompile(`^([A-Z][a-z0-9]+)+$`)
	rePtWord = regexp.MustCompile(`(?i)\b(que|não|para|uma|com|quando|arquivo|função|usuário|então|também)\b`)
	reEnWord = regexp.MustCompile(`(?i)\b(the|and|when|file|function|user|this|should|returns?)\b`)
)

type Tally map[string]int

func (t Tally) Top() (string, int, int) {
	total, best, n := 0, "", 0
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		total += t[k]
		if t[k] > n {
			best, n = k, t[k]
		}
	}
	return best, n, total
}

func pct(n, total int) string { return fmt.Sprintf("%d%%", n*100/max(total, 1)) }

var (
	jsLikeExts = map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true, ".vue": true, ".svelte": true}
	srcExts    = map[string]bool{".go": true, ".py": true, ".rs": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true, ".css": true, ".scss": true, ".html": true}
)

type conventionScan struct {
	indent, quotes, semis, modules, eol, lang, tests Tally
	naming                                           map[string]Tally
	lengths                                          []int
	files                                            int
}

// LearnConventions analyses the project and returns the rules as Markdown.
func LearnConventions(root string) string {
	sc := &conventionScan{
		indent: Tally{}, quotes: Tally{}, semis: Tally{}, modules: Tally{}, eol: Tally{}, lang: Tally{}, tests: Tally{},
		naming: map[string]Tally{},
	}
	_ = fsx.Walk(root, fsx.Project, sc.visit)
	if sc.files == 0 {
		return ""
	}
	rules := sc.rules()
	_ = os.MkdirAll(filepath.Join(root, core.InfuseDirName), 0o755)
	_ = core.WriteJSON(ConventionRulesFile(root), rules)
	return sc.markdown()
}

func (sc *conventionScan) visit(p string, d os.DirEntry, err error) error {
	if err != nil || sc.files >= 400 {
		return nil
	}
	if d.IsDir() {
		if d.Name() == "tests" || d.Name() == "test" || d.Name() == "__tests__" {
			sc.tests["folder "+d.Name()+"/"]++
		}
		return nil
	}
	ext := strings.ToLower(filepath.Ext(p))
	if !jsLikeExts[ext] && !srcExts[ext] {
		return nil
	}
	sc.fileName(d.Name(), ext)
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 400<<10 {
		return nil
	}
	sc.files++
	sc.fileText(string(b), ext)
	return nil
}

func (sc *conventionScan) fileName(file, ext string) {
	name := strings.TrimSuffix(file, filepath.Ext(file))
	switch {
	case strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec"):
		sc.tests["*"+name[strings.LastIndex(name, "."):]+ext]++
		name = name[:strings.LastIndex(name, ".")]
	case strings.HasSuffix(name, "_test") && ext == ".go":
		sc.tests["*_test.go"]++
	case strings.HasPrefix(name, "test_") && ext == ".py":
		sc.tests["test_*.py"]++
	}
	if sc.naming[ext] == nil {
		sc.naming[ext] = Tally{}
	}
	switch {
	case ReKebab.MatchString(name):
		sc.naming[ext]["kebab-case"]++
	case ReSnake.MatchString(name):
		sc.naming[ext]["snake_case"]++
	case ReCamel.MatchString(name):
		sc.naming[ext]["camelCase"]++
	case RePascal.MatchString(name):
		sc.naming[ext]["PascalCase"]++
	}
}

func (sc *conventionScan) fileText(text, ext string) {
	if strings.Contains(text, "\r\n") {
		sc.eol["CRLF"]++
	} else {
		sc.eol["LF"]++
	}
	fileIndent := Tally{}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		sc.lengths = append(sc.lengths, len([]rune(line)))
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		sc.lineIndent(fileIndent, line, trimmed)
		sc.lineComment(trimmed)
		if jsLikeExts[ext] {
			sc.lineJS(trimmed)
		}
	}
	if k, _, _ := fileIndent.Top(); k != "" {
		sc.indent[k]++
	}
}

func (sc *conventionScan) lineIndent(tally Tally, line, trimmed string) {
	lead := line[:len(line)-len(trimmed)]
	if strings.HasPrefix(lead, "\t") { // indentation, on indented lines only
		tally["tabs"]++
	} else if n := len(lead); n%4 == 0 && n > 0 {
		tally["4 spaces"]++
	} else if n%2 == 0 && n > 0 {
		tally["2 spaces"]++
	}
}

func (sc *conventionScan) lineComment(trimmed string) {
	c := strings.TrimSpace(trimmed)
	if !strings.HasPrefix(c, "//") && !strings.HasPrefix(c, "#") && !strings.HasPrefix(c, "/*") && !strings.HasPrefix(c, "*") {
		return
	}
	if rePtWord.MatchString(c) {
		sc.lang["Portuguese"]++
	} else if reEnWord.MatchString(c) {
		sc.lang["English"]++
	}
}

func (sc *conventionScan) lineJS(trimmed string) {
	if strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "require(") {
		if strings.Contains(trimmed, "'") {
			sc.quotes["single quotes"]++
		} else if strings.Contains(trimmed, `"`) {
			sc.quotes["double quotes"]++
		}
		if strings.HasPrefix(trimmed, "import ") {
			sc.modules["ESM (import/export)"]++
		} else {
			sc.modules["CommonJS (require)"]++
		}
	}
	if strings.HasSuffix(trimmed, ";") {
		sc.semis["with semicolons"]++
	} else if strings.HasSuffix(trimmed, ")") || strings.HasSuffix(trimmed, "'") || strings.HasSuffix(trimmed, `"`) {
		sc.semis["without semicolons"]++
	}
}

func (sc *conventionScan) rules() ConventionRules {
	// the enforceable rules, with the share of samples behind each (protocol.go refuses writes against them)
	rules := ConventionRules{Files: sc.files, Naming: map[string]string{}, NamingShare: map[string]float64{}, NamingN: map[string]int{}}
	share := func(t Tally) (string, float64) {
		k, n, total := t.Top()
		if total == 0 {
			return "", 0
		}
		return k, float64(n) / float64(total)
	}
	if k, sh := share(sc.indent); k != "" {
		rules.Indent, rules.IndentShare = map[string]string{"tabs": "tabs", "2 spaces": "2", "4 spaces": "4"}[k], sh
	}
	if k, sh := share(sc.quotes); k != "" {
		rules.Quotes, rules.QuotesShare = map[string]string{"single quotes": "single", "double quotes": "double"}[k], sh
	}
	if k, sh := share(sc.semis); k != "" {
		rules.Semis, rules.SemisShare = map[string]string{"with semicolons": "with", "without semicolons": "without"}[k], sh
	}
	if k, sh := share(sc.modules); k != "" {
		rules.Modules, rules.ModShare = map[string]string{"ESM (import/export)": "esm", "CommonJS (require)": "cjs"}[k], sh
	}
	for e, t := range sc.naming {
		if k, n, total := t.Top(); k != "" {
			rules.Naming[e], rules.NamingShare[e], rules.NamingN[e] = k, float64(n)/float64(total), total
		}
	}
	return rules
}

func (sc *conventionScan) markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Project conventions\n\nDetected in %d code files (counted, not assumed). Edit freely: the agent reads this file on every turn.\n\n", sc.files)
	line := func(label string, t Tally) {
		if k, n, total := t.Top(); k != "" && total > 0 {
			fmt.Fprintf(&b, "- %s: %s (%s)\n", label, k, pct(n, total))
		}
	}
	line("Indentation", sc.indent)
	line("Quotes in imports", sc.quotes)
	line("End of statement (JS/TS)", sc.semis)
	line("Modules", sc.modules)
	line("Line endings", sc.eol)
	if k, n, total := sc.lang.Top(); k != "" && total > 0 {
		fmt.Fprintf(&b, "\n## Observed, not a rule\n- Comments are mostly in %s (%s of files). This is a fact about the existing code, not a convention: new code follows the project's configured language.\n", k, pct(n, total))
	}
	exts := make([]string, 0, len(sc.naming))
	for e := range sc.naming {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	for _, e := range exts {
		if k, n, total := sc.naming[e].Top(); k != "" && total >= 3 {
			fmt.Fprintf(&b, "- File names %s: %s (%s)\n", e, k, pct(n, total))
		}
	}
	if len(sc.tests) > 0 {
		keys := make([]string, 0, len(sc.tests))
		for k := range sc.tests {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "- Tests: %s\n", strings.Join(keys, ", "))
	}
	if len(sc.lengths) > 20 {
		sort.Ints(sc.lengths)
		fmt.Fprintf(&b, "- Line length: 95%% of lines up to %d characters\n", sc.lengths[len(sc.lengths)*95/100])
	}
	return b.String()
}

// conventionsPrompt returns the project's conventions, learning them the first time.
func ConventionsPrompt(root string) string {
	p := ConventionsFile(root)
	if b, err := os.ReadFile(p); err == nil {
		return string(b)
	}
	text := LearnConventions(root)
	if text != "" {
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(text), 0o644)
	}
	return text
}
