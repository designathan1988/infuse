package agent

// Locks against gaming the checks (docs/FALHAS-DE-AGENTES.md B6, B7, B8, B10). Every signal is code
// syntax, never natural language: a test command whose failure is swallowed, the expected values of the
// tests appearing in production conditions, equality or clocks that always agree, test infrastructure
// changed, functions hollowed into stubs, new TODO/FIXME/HACK markers.

import (
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	reTestRunner   = regexp.MustCompile(`\b(?:go\s+test|npm\s+(?:run\s+)?test|pnpm\s+(?:run\s+)?test|yarn\s+(?:run\s+)?test|npx\s+(?:jest|vitest|mocha|playwright)|jest|vitest|mocha|pytest|python3?\s+-m\s+(?:pytest|unittest)|node\s+--test|cargo\s+test|dotnet\s+test|mvn\s+test|gradle\w*\s+test|deno\s+test|bun\s+test)\b`)
	reSwallowExit  = regexp.MustCompile(`\|\|\s*(?:true|:|exit\s+0)\b|;\s*(?:true|exit\s+0)\s*$|\bset\s+\+e\b`)
	reTestInfra    = regexp.MustCompile(`(?i)(?:^|/)(?:conftest\.py|sitecustomize\.py|usercustomize\.py|[^/]*\.pth|jest\.(?:config|setup)[^/]*|vitest\.(?:config|setup|workspace)[^/]*|setuptests\.[jt]sx?|karma\.conf[^/]*|\.mocharc[^/]*|pytest\.ini|tox\.ini|playwright\.config[^/]*)$`)
	reAlwaysEqual  = regexp.MustCompile(`(?s)def\s+__(?:eq|ne)__\s*\([^)]*\)\s*(?:->\s*\w+\s*)?:\s*\n\s*return\s+(?:True|False)\b|\bequals\s*\([^)]*\)\s*(?::\s*\w+\s*)?\{\s*return\s+(?:true|false)\s*;?\s*\}`)
	reClockPatch   = regexp.MustCompile(`\b(?:time\.time|time\.perf_counter|time\.monotonic|Date\.now|performance\.now|process\.hrtime)\s*=[^=]`)
	reMarker       = regexp.MustCompile(`\b(?:TODO|FIXME|HACK|XXX)\b`)
	reStubBody     = regexp.MustCompile(`^\s*(?:pass|\.\.\.|return\s*(?:null|None|nil|undefined|0|false|False|""|''|\[\]|\{\})?\s*;?|throw\s+new\s+\w*Error\([^)]*\)\s*;?|raise\s+NotImplementedError\b.*|panic\([^)]*\)|unimplemented!\(\)|todo!\(\))\s*$`)
	reStringLit    = regexp.MustCompile(`"([^"\\\n]{4,80})"|'([^'\\\n]{4,80})'`)
	reNumberLit    = regexp.MustCompile(`\b\d{3,}(?:\.\d+)?\b`)
	reConditionPos = regexp.MustCompile(`\b(?:if|elif|else\s+if|case|when|switch)\b|===?|!==?|\?\s`)
)

func isTestPath(rel string) bool { return reTestFile.MatchString(filepath.ToSlash(rel)) }

// swallowedTestCommand: a test run whose failure is made to look like success.
func swallowedTestCommand(cmd string) Msg {
	if reTestRunner.MatchString(cmd) && reSwallowExit.MatchString(cmd) {
		return M("cheat.swallowed_exit", "command", truncate(strings.TrimSpace(cmd), 160))
	}
	return Msg{}
}

// testLiterals collects the expected values written in the project's tests (strings and long numbers).
func testLiterals(work string) map[string]string {
	out := map[string]string{}
	walkProjectFiles(work, func(rel, abs string, size int64) {
		if !isTestPath(rel) || size > 300<<10 || !codeExts[strings.ToLower(path.Ext(rel))] {
			return
		}
		b, err := readFileLimited(abs, 300<<10)
		if err != nil {
			return
		}
		for _, m := range reStringLit.FindAllStringSubmatch(b, -1) {
			v := m[1] + m[2]
			if strings.TrimSpace(v) != "" && !strings.ContainsAny(v, "/\\") { // paths and imports are not expected values
				out[v] = rel
			}
		}
		for _, n := range reNumberLit.FindAllString(b, -1) {
			out[n] = rel
		}
	})
	return out
}

// cheatWriteCheck looks at what a write adds to a file.
func cheatWriteCheck(work, rel, before, after string) Msg {
	rel = filepath.ToSlash(rel)
	ext := strings.ToLower(path.Ext(rel))
	added := addedLines(before, after)
	addedText := strings.Join(added, "\n")
	// B8: infrastructure that decides whether tests pass
	if reTestInfra.MatchString(rel) && before != after {
		return M("cheat.test_infra", "file", rel)
	}
	// B8: equality or clocks forced
	if reAlwaysEqual.MatchString(addedText) && !reAlwaysEqual.MatchString(before) {
		return M("cheat.always_equal", "file", rel)
	}
	if !isTestPath(rel) && reClockPatch.MatchString(stripCode(addedText, ext)) {
		return M("cheat.clock_patch", "file", rel)
	}
	if !codeExts[ext] || ext == ".css" || ext == ".html" {
		return Msg{}
	}
	if m := cheatMarkers(rel, ext, before, after); !m.Empty() {
		return m
	}
	// B7: the tests' expected values used as conditions in production code
	if isTestPath(rel) {
		return Msg{}
	}
	return cheatSpecialCase(work, rel, added)
}

// cheatMarkers catches unfinished-work markers and a function hollowed into a stub.
func cheatMarkers(rel, ext, before, after string) Msg {
	// B10: markers of unfinished work
	if n, o := len(reMarker.FindAllString(after, -1)), len(reMarker.FindAllString(before, -1)); n > o {
		return M("cheat.markers", "file", rel, "count", n-o)
	}
	// B10: a function hollowed into a stub
	old := map[string]funcMetric{}
	for _, f := range functionMetrics(before, ext) {
		old[f.name] = f
	}
	lines := strings.Split(after, "\n")
	for _, f := range functionMetrics(after, ext) {
		o, had := old[f.name]
		if !had || o.lines < 5 || f.lines > 3 {
			continue
		}
		body := strings.Join(lines[f.line-1:min(len(lines), f.line-1+f.lines)], "\n")
		inner := body
		if i, j := strings.Index(body, "{"), strings.LastIndex(body, "}"); i >= 0 && j > i {
			inner = body[i+1 : j]
		} else if i := strings.Index(body, ":"); ext == ".py" && i >= 0 {
			inner = body[i+1:]
		}
		if reStubBody.MatchString(strings.TrimSpace(inner)) || strings.TrimSpace(inner) == "" {
			return M("cheat.stub", "function", f.name, "file", rel, "before", o.lines)
		}
	}
	return Msg{}
}

func cheatSpecialCase(work, rel string, added []string) Msg {
	lits := testLiterals(work)
	found := map[string]string{}
	for _, code := range added {
		if !reConditionPos.MatchString(code) {
			continue
		}
		for _, m := range reStringLit.FindAllStringSubmatch(code, -1) {
			if t, ok := lits[m[1]+m[2]]; ok {
				found[m[1]+m[2]] = t
			}
		}
		for _, n := range reNumberLit.FindAllString(code, -1) {
			if t, ok := lits[n]; ok {
				found[n] = t
			}
		}
	}
	if len(found) < 2 {
		return Msg{}
	}
	var vals, tests []string
	for v, t := range found {
		vals = append(vals, v)
		if !contains(tests, t) {
			tests = append(tests, t)
		}
	}
	sort.Strings(vals)
	return M("cheat.special_case", "file", rel, "values", strings.Join(firstN(vals, 5), ", "), "tests", strings.Join(tests, ", "))
}

// readFileLimited reads at most limit bytes of a file as text.
func readFileLimited(abs string, limit int64) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit))
	return string(b), err
}
