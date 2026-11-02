package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Grep: regex search with context.

// ---------------------------------------------------------------- Grep

type grepTool struct{}

func (grepTool) Category() string { return "read" }
func (grepTool) Spec() ToolSpec {
	return ToolSpec{Name: "Grep", Description: "Searches file contents with a regular expression. Returns file:line:text. count=true returns how many lines match in each file; type filters by language (go, js, ts, py, rust, ...).",
		InputSchema: schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string","description":"Filter files, e.g. *.ts"},"ignore_case":{"type":"boolean"},"context":{"type":"integer","description":"Lines of context"},"files_only":{"type":"boolean"},"count":{"type":"boolean","description":"Matching lines per file instead of the lines"},"type":{"type":"string","description":"Language filter, e.g. go, ts, py"}},"required":["pattern"]}`)}
}

type grepInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	Glob       string `json:"glob"`
	IgnoreCase bool   `json:"ignore_case"`
	Context    int    `json:"context"`
	FilesOnly  bool   `json:"files_only"`
	Count      bool   `json:"count"`
	Type       string `json:"type"`
}

func (grepTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in grepInput
	_ = json.Unmarshal(input, &in)
	pat := in.Pattern
	if in.IgnoreCase {
		pat = "(?i)" + pat
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return tc.Say(M("grep.bad_regex", "error", err.Error())), true
	}
	if in.Type != "" && !grepTypeKnown(in.Type) {
		return tc.Say(M("grep.bad_type", "type", in.Type)), true
	}
	base, err := tc.walkBase(in.Path)
	if err != nil {
		return tc.SayErr(err), true
	}
	hidden := tc.walkFilter()
	var fileRe *regexp.Regexp
	if in.Glob != "" {
		fileRe = globToRegex("**/" + in.Glob)
	}
	var sb strings.Builder
	matches := 0
	_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil || matches > 500 && !in.Count { // counting reads everything: its output stays short
			return nil
		}
		if hidden(p, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != base && ignoredPath(tc.Root, p) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(tc.Root, p)
		grepFile(p, toPosix(rel), re, fileRe, &in, &sb, &matches)
		return nil
	})
	if matches == 0 {
		return tc.Say(M("grep.none")), false
	}
	if in.Count {
		return tc.clip(sb.String() + tc.Say(M("grep.total", "count", matches)) + "\n"), false
	}
	if m := LoadMap(tc.Root); m != nil { // the same directory context LS gives
		if d := DescribeDir(m, relTo(tc.Root, base)); d != "" {
			sb.WriteString("\n" + d + "\n")
		}
	}
	return tc.clip(sb.String()), false
}

func grepFile(p, rel string, re, fileRe *regexp.Regexp, in *grepInput, sb *strings.Builder, matches *int) {
	if fileRe != nil && !fileRe.MatchString(rel) || !grepTypeMatches(in.Type, rel) {
		return
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) > 2_000_000 || bytes.IndexByte(b, 0) >= 0 {
		return
	}
	lines := strings.Split(string(b), "\n")
	fileHit, inFile := false, 0
	defer func() {
		if in.Count && inFile > 0 {
			fmt.Fprintf(sb, "%s:%d\n", rel, inFile)
		}
	}()
	for i, l := range lines {
		if !re.MatchString(l) {
			continue
		}
		*matches++
		inFile++
		if in.Count {
			continue
		}
		if in.FilesOnly {
			if !fileHit {
				sb.WriteString(rel + "\n")
			}
			fileHit = true
			continue
		}
		grepContext(sb, rel, lines, i, in.Context)
	}
}

func grepContext(sb *strings.Builder, rel string, lines []string, i, context int) {
	from, to := i-context, i+context
	if from < 0 {
		from = 0
	}
	if to >= len(lines) {
		to = len(lines) - 1
	}
	for j := from; j <= to; j++ {
		sep := "-"
		if j == i {
			sep = ":"
		}
		fmt.Fprintf(sb, "%s:%d%s%s\n", rel, j+1, sep, lines[j])
	}
	if context > 0 {
		sb.WriteString("--\n")
	}
}
