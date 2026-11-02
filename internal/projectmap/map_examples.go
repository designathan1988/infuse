package projectmap

// Level "how X is done here": for each common operation, the shortest real file that does it, at the
// line that does it. The operations and their patterns are a data table (internal/data).

import (
	"encoding/json"
	"infuse/internal/data"
	"infuse/internal/lang"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Example is one real occurrence of an operation.
type Example struct {
	Op    string `json:"op"`
	Label string `json:"label"`
	Lang  string `json:"lang"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
	lines int
}

type exampleOp struct {
	Op       string            `json:"op"`
	Label    string            `json:"label"`
	Tests    bool              `json:"tests,omitempty"` // this operation is only found in test files
	Patterns map[string]string `json:"patterns"`
	compiled map[string]*regexp.Regexp
}

var loadExampleOps = sync.OnceValue(func() []exampleOp {
	var ops []exampleOp
	_ = json.Unmarshal(data.Read("map_examples.json"), &ops)
	for i := range ops {
		ops[i].compiled = map[string]*regexp.Regexp{}
		for l, p := range ops[i].Patterns {
			ops[i].compiled[l] = regexp.MustCompile(p)
		}
	}
	return ops
})

// exampleLang folds the languages that share one pattern.
func exampleLang(l string) string {
	if l == "js" {
		return "ts"
	}
	return l
}

// computeExamples keeps, per operation and language, the shortest file with a match.
func computeExamples(m *ProjectMap) []Example {
	best := map[string]Example{}
	for _, f := range sortedFiles(m) {
		if f.scan == nil || nonCodeLangs[f.Lang] {
			continue
		}
		isTest := lang.IsTestPath(f.Path)
		for _, op := range loadExampleOps() {
			re := op.compiled[exampleLang(f.Lang)]
			if re == nil || op.Tests != isTest {
				continue
			}
			key := op.Op + "|" + exampleLang(f.Lang)
			if cur, ok := best[key]; ok && cur.lines <= f.Lines {
				continue
			}
			if loc := re.FindStringIndex(f.scan.text); loc != nil {
				best[key] = exampleAt(op, f, loc[0])
			}
		}
	}
	out := make([]Example, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Op != out[j].Op {
			return out[i].Op < out[j].Op
		}
		return out[i].Lang < out[j].Lang
	})
	return out
}

func exampleAt(op exampleOp, f *FileInfo, at int) Example {
	text := f.scan.text
	start := strings.LastIndexByte(text[:at], '\n') + 1
	end := len(text)
	if n := strings.IndexByte(text[at:], '\n'); n >= 0 {
		end = at + n
	}
	line := strings.TrimSpace(text[start:end])
	if len(line) > 120 {
		line = line[:120]
	}
	return Example{Op: op.Op, Label: op.Label, Lang: exampleLang(f.Lang), File: f.Path, Line: strings.Count(text[:at], "\n") + 1, Text: line, lines: f.Lines}
}
