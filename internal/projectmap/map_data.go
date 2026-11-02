package projectmap

// Level "data → owner": the types and stores a project declares, and the files that mutate them. One
// writer is an owner; several writers are a shared datum, stated as such. Regular expressions over the
// text: a variable is tied to its type where the file declares it, so an alias passed around is not
// followed.

import (
	"regexp"
	"sort"
	"strings"
)

// DataOwner is one datum. Owner is set only when a single file writes it.
type DataOwner struct {
	Name    string   `json:"name"`
	Kind    string   `json:"kind"` // type | store
	File    string   `json:"file"`
	Line    int      `json:"line"`
	Owner   string   `json:"owner,omitempty"`
	Writers []string `json:"writers"`
}

// maxDataText bounds the text read per file for this level.
const maxDataText = 256 << 10

// maxDataRows bounds the level: shared data first, since that is what needs an owner.
const maxDataRows = 400

type dataDecl struct {
	name, kind string
	line       int
}

// fileScan is what one file contributes to the levels of a full build.
type fileScan struct {
	decls    []dataDecl
	assigned map[string]bool   // names written to in this file
	vars     map[string]string // variable → declared type
	tokens   map[string]int    // identifiers used, for the dead-code level
	text     string            // bounded text, for the examples
}

type dataPatterns struct {
	types, stores []*regexp.Regexp
}

var dataTable = map[string]dataPatterns{
	"go": {types: res(`(?m)^type\s+(\w+)\s+struct\b`), stores: res(`(?m)^var\s+(\w+)\b`)},
	"ts": {
		types:  res(`(?m)^(?:export\s+)?(?:default\s+)?(?:abstract\s+)?(?:class|interface)\s+(\w+)`, `(?m)^(?:export\s+)?type\s+(\w+)\s*=\s*\{`),
		stores: res(`(?m)^(?:export\s+)?(?:let|var)\s+(\w+)`, `(?m)^(?:export\s+)?const\s+(\w+)\s*(?::[^=]+)?=\s*(?:new\s+(?:Map|Set|WeakMap)\b|\[|\{|\w*[sS]tore\(|writable\(|ref\(|reactive\(|atom\()`),
	},
	"py": {types: res(`(?m)^class\s+(\w+)`), stores: res(`(?m)^(\w+)\s*(?::[^=\n]+)?=\s*(?:\{|\[|dict\(|list\(|set\(|defaultdict\()`)},
	"rs": {types: res(`(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?struct\s+(\w+)`), stores: res(`(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?static\s+(?:mut\s+)?(\w+)`)},
	"cs": {types: res(`(?m)^\s*(?:(?:public|internal|sealed|abstract|partial)\s+)*(?:class|record|struct)\s+(\w+)`), stores: res(`(?m)^\s*(?:(?:public|internal|private|protected)\s+)?static\s+(?:readonly\s+)?[\w<>\[\],]+\s+(\w+)\s*[=;]`)},
}

func res(patterns ...string) []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		out = append(out, regexp.MustCompile(p))
	}
	return out
}

var (
	reDeclLine = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:var|let|const|static)\b.*$`)
	reWrite    = regexp.MustCompile(`(?:^|[\s({;,])([A-Za-z_$][\w$]*)(?:\.[\w$]+|\[[^\]\n]*\])*\s*(?:=[^=>~]|\+=|-=|\*=|\|=|&=|\+\+|--)`)
	reMutCall  = regexp.MustCompile(`\b([A-Za-z_$][\w$]*)\.(?:push|pop|shift|unshift|splice|set|add|delete|clear|append|extend|insert|remove|update|sort|Store|Delete|Add|Put|Set)\(`)
	reGoDelete = regexp.MustCompile(`\bdelete\(\s*([A-Za-z_]\w*)\b`)
	reVarNew   = regexp.MustCompile(`\b(\w+)\s*(?::=|=)\s*&?(?:new\s+)?([A-Z]\w*)\s*[{(]`)
	reVarColon = regexp.MustCompile(`\b(\w+)\s*:\s*&?(?:mut\s+)?([A-Z]\w*)\b`)
	reVarGo    = regexp.MustCompile(`[(,]\s*(\w+)\s+\*?([A-Z]\w*)\s*[,)]`)
	reVarDecl  = regexp.MustCompile(`(?m)^\s*(?:var\s+(\w+)\s+\*?([A-Z]\w*)\b)`)
)

var dataLangAlias = map[string]string{"js": "ts", "vue": "ts", "svelte": "ts"}

func dataLang(lang string) string {
	if a, ok := dataLangAlias[lang]; ok {
		return a
	}
	return lang
}

// scanFile reads what the levels need from one file.
func scanFile(f *FileInfo, text string) *fileScan {
	if len(text) > maxDataText {
		text = text[:maxDataText]
	}
	s := &fileScan{assigned: map[string]bool{}, vars: map[string]string{}, tokens: tokenCounts(text), text: text}
	if p, ok := dataTable[dataLang(f.Lang)]; ok {
		s.decls = append(declared(text, p.types, "type"), declared(text, p.stores, "store")...)
		collectWrites(s, text)
	}
	return s
}

func declared(text string, patterns []*regexp.Regexp, kind string) []dataDecl {
	var out []dataDecl
	for _, re := range patterns {
		for _, loc := range re.FindAllStringSubmatchIndex(text, -1) {
			out = append(out, dataDecl{text[loc[2]:loc[3]], kind, strings.Count(text[:loc[0]], "\n") + 1})
		}
	}
	return out
}

func collectWrites(s *fileScan, text string) {
	body := reDeclLine.ReplaceAllString(text, "") // a declaration's initial value is not a write
	for _, re := range []*regexp.Regexp{reWrite, reMutCall, reGoDelete} {
		addAll(s.assigned, re, body)
	}
	for _, re := range []*regexp.Regexp{reVarNew, reVarColon, reVarGo, reVarDecl} {
		for _, mm := range re.FindAllStringSubmatch(text, -1) {
			if s.vars[mm[1]] == "" {
				s.vars[mm[1]] = mm[2]
			}
		}
	}
}

// computeData joins the declarations with the writes of every file.
func computeData(m *ProjectMap) []DataOwner {
	byType := map[string][]string{}  // type → files that write through a variable of it
	byStore := map[string][]string{} // name → files that assign to it
	files := sortedFiles(m)
	for _, f := range files {
		if f.scan == nil {
			continue
		}
		for name := range f.scan.assigned {
			byStore[name] = append(byStore[name], f.Path)
		}
		seen := map[string]bool{}
		for v, t := range f.scan.vars {
			if f.scan.assigned[v] && !seen[t] {
				seen[t] = true
				byType[t] = append(byType[t], f.Path)
			}
		}
	}
	var out []DataOwner
	for _, f := range files {
		if f.scan == nil {
			continue
		}
		for _, d := range f.scan.decls {
			if row, ok := dataRow(f, d, byType, byStore); ok {
				out = append(out, row)
			}
		}
	}
	return boundedData(out)
}

// dataRow keeps a datum something writes; a type with no writer is a shape, a store with none a table.
func dataRow(f *FileInfo, d dataDecl, byType, byStore map[string][]string) (DataOwner, bool) {
	writers := append([]string{}, byStore[d.name]...)
	if d.kind == "type" {
		writers = append([]string{}, byType[d.name]...)
		if f.scan.assigned["this"] || f.scan.assigned["self"] {
			writers = append(writers, f.Path) // a class that mutates itself
		}
	}
	if len(writers) == 0 {
		return DataOwner{}, false
	}
	sort.Strings(writers)
	writers = dedupe(writers)
	row := DataOwner{Name: d.name, Kind: d.kind, File: f.Path, Line: d.line, Writers: writers}
	if len(writers) == 1 {
		row.Owner = writers[0]
	}
	return row, true
}

func boundedData(rows []DataOwner) []DataOwner {
	sort.Slice(rows, func(i, j int) bool {
		si, sj := rows[i].Owner == "", rows[j].Owner == ""
		if si != sj {
			return si
		}
		if rows[i].Name != rows[j].Name {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].File < rows[j].File
	})
	if len(rows) > maxDataRows {
		rows = rows[:maxDataRows]
	}
	return rows
}
