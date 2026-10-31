package knowledge

// Convention learner: reads a sample of the project's source files and states its conventions
// (indentation, quotes, semicolons, module system, file naming, test layout, line endings, comment
// language) as short rules in .infuse/conventions.md. Counting, not guessing: every rule comes with the
// share of files it was seen in. The user can edit the file; the agent reads it every turn.

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/projectmap"
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

func ConventionsFile(root string) string { return filepath.Join(root, core.InfuseDirName, "conventions.md") }

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

// LearnConventions analyses the project and returns the rules as Markdown.
func LearnConventions(root string) string {
	indent, quotes, semis, modules, naming, eol, lang := Tally{}, Tally{}, Tally{}, Tally{}, map[string]Tally{}, Tally{}, Tally{}
	tests := Tally{}
	var lengths []int
	files := 0
	jsLike := map[string]bool{".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".mjs": true, ".cjs": true, ".vue": true, ".svelte": true}
	src := map[string]bool{".go": true, ".py": true, ".rs": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true, ".css": true, ".scss": true, ".html": true}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || files >= 400 {
			return nil
		}
		if d.IsDir() {
			if p != root && (projectmap.IgnoredDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			if d.Name() == "tests" || d.Name() == "test" || d.Name() == "__tests__" {
				tests["folder "+d.Name()+"/"]++
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !jsLike[ext] && !src[ext] {
			return nil
		}
		name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
		switch {
		case strings.HasSuffix(name, ".test") || strings.HasSuffix(name, ".spec"):
			tests["*"+name[strings.LastIndex(name, "."):]+ext]++
			name = name[:strings.LastIndex(name, ".")]
		case strings.HasSuffix(name, "_test") && ext == ".go":
			tests["*_test.go"]++
		case strings.HasPrefix(name, "test_") && ext == ".py":
			tests["test_*.py"]++
		}
		if naming[ext] == nil {
			naming[ext] = Tally{}
		}
		switch {
		case ReKebab.MatchString(name):
			naming[ext]["kebab-case"]++
		case ReSnake.MatchString(name):
			naming[ext]["snake_case"]++
		case ReCamel.MatchString(name):
			naming[ext]["camelCase"]++
		case RePascal.MatchString(name):
			naming[ext]["PascalCase"]++
		}
		b, err := os.ReadFile(p)
		if err != nil || len(b) > 400<<10 {
			return nil
		}
		files++
		text := string(b)
		if strings.Contains(text, "\r\n") {
			eol["CRLF"]++
		} else {
			eol["LF"]++
		}
		fileIndent := Tally{}
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			lengths = append(lengths, len([]rune(line)))
			trimmed := strings.TrimLeft(line, " \t")
			if trimmed == "" {
				continue
			}
			if lead := line[:len(line)-len(trimmed)]; strings.HasPrefix(lead, "\t") { // indentation, on indented lines only
				fileIndent["tabs"]++
			} else if n := len(lead); n%4 == 0 && n > 0 {
				fileIndent["4 spaces"]++
			} else if n%2 == 0 && n > 0 {
				fileIndent["2 spaces"]++
			}
			if c := strings.TrimSpace(trimmed); strings.HasPrefix(c, "//") || strings.HasPrefix(c, "#") || strings.HasPrefix(c, "/*") || strings.HasPrefix(c, "*") {
				if rePtWord.MatchString(c) {
					lang["Portuguese"]++
				} else if reEnWord.MatchString(c) {
					lang["English"]++
				}
			}
			if jsLike[ext] {
				if strings.HasPrefix(trimmed, "import ") || strings.Contains(trimmed, "require(") {
					if strings.Contains(trimmed, "'") {
						quotes["single quotes"]++
					} else if strings.Contains(trimmed, `"`) {
						quotes["double quotes"]++
					}
					if strings.HasPrefix(trimmed, "import ") {
						modules["ESM (import/export)"]++
					} else {
						modules["CommonJS (require)"]++
					}
				}
				if strings.HasSuffix(trimmed, ";") {
					semis["with semicolons"]++
				} else if strings.HasSuffix(trimmed, ")") || strings.HasSuffix(trimmed, "'") || strings.HasSuffix(trimmed, `"`) {
					semis["without semicolons"]++
				}
			}
		}
		if k, _, _ := fileIndent.Top(); k != "" {
			indent[k]++
		}
		return nil
	})
	if files == 0 {
		return ""
	}
	// the enforceable rules, with the share of samples behind each (protocol.go refuses writes against them)
	rules := ConventionRules{Files: files, Naming: map[string]string{}, NamingShare: map[string]float64{}, NamingN: map[string]int{}}
	share := func(t Tally) (string, float64) {
		k, n, total := t.Top()
		if total == 0 {
			return "", 0
		}
		return k, float64(n) / float64(total)
	}
	if k, sh := share(indent); k != "" {
		rules.Indent, rules.IndentShare = map[string]string{"tabs": "tabs", "2 spaces": "2", "4 spaces": "4"}[k], sh
	}
	if k, sh := share(quotes); k != "" {
		rules.Quotes, rules.QuotesShare = map[string]string{"single quotes": "single", "double quotes": "double"}[k], sh
	}
	if k, sh := share(semis); k != "" {
		rules.Semis, rules.SemisShare = map[string]string{"with semicolons": "with", "without semicolons": "without"}[k], sh
	}
	if k, sh := share(modules); k != "" {
		rules.Modules, rules.ModShare = map[string]string{"ESM (import/export)": "esm", "CommonJS (require)": "cjs"}[k], sh
	}
	for e, t := range naming {
		if k, n, total := t.Top(); k != "" {
			rules.Naming[e], rules.NamingShare[e], rules.NamingN[e] = k, float64(n)/float64(total), total
		}
	}
	_ = os.MkdirAll(filepath.Join(root, core.InfuseDirName), 0o755)
	_ = core.WriteJSON(ConventionRulesFile(root), rules)
	var b strings.Builder
	fmt.Fprintf(&b, "# Project conventions\n\nDetected in %d code files (counted, not assumed). Edit freely: the agent reads this file on every turn.\n\n", files)
	line := func(label string, t Tally) {
		if k, n, total := t.Top(); k != "" && total > 0 {
			fmt.Fprintf(&b, "- %s: %s (%s)\n", label, k, pct(n, total))
		}
	}
	line("Indentation", indent)
	line("Quotes in imports", quotes)
	line("End of statement (JS/TS)", semis)
	line("Modules", modules)
	line("Line endings", eol)
	line("Comment language", lang)
	exts := make([]string, 0, len(naming))
	for e := range naming {
		exts = append(exts, e)
	}
	sort.Strings(exts)
	for _, e := range exts {
		if k, n, total := naming[e].Top(); k != "" && total >= 3 {
			fmt.Fprintf(&b, "- File names %s: %s (%s)\n", e, k, pct(n, total))
		}
	}
	if len(tests) > 0 {
		keys := make([]string, 0, len(tests))
		for k := range tests {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintf(&b, "- Tests: %s\n", strings.Join(keys, ", "))
	}
	if len(lengths) > 20 {
		sort.Ints(lengths)
		fmt.Fprintf(&b, "- Line length: 95%% of lines up to %d characters\n", lengths[len(lengths)*95/100])
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
