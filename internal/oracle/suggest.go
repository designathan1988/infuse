package oracle

// Ready-made criteria for "Ready?", read from what the project already declares (package.json scripts,
// go.mod, Cargo.toml, pyproject, .NET, Maven/Gradle, Makefile) so the user adds them with one click
// instead of writing commands from memory.

import (
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type CheckSuggestion struct {
	Title    string      `json:"title"` // what must be true, in plain words (in the user's language)
	Why      string      `json:"why"`
	TitleMsg core.Msg    `json:"titleMsg"` // the same, as messages of the catalog
	WhyMsg   core.Msg    `json:"whyMsg"`
	Check    OracleCheck `json:"check"`
	Setup    string      `json:"setup,omitempty"` // preparation the check needs in the snapshot (install dependencies)
}

func hasPath(root, rel string) bool { _, err := os.Stat(filepath.Join(root, rel)); return err == nil }

func cmdCheck(id, title, run string) OracleCheck {
	zero := 0
	c := OracleCheck{ID: id, Kind: "command", Run: run, Description: title}
	c.Expect.Exit = &zero
	return c
}

var reMakeTest = regexp.MustCompile(`(?m)^test\s*:`)

// The suggestion's Title and Why are messages written in the user's language; the check's Description
// is stored in the criteria the model reads, so it is English.

// SuggestChecks lists criteria that fit the project at root, leaving out those already in the spec.
func SuggestChecks(root string) []CheckSuggestion {
	s := &suggester{root: root, lang: config.RootLanguage(root)}
	s.node()
	s.goProject()
	s.otherLanguages()
	s.sizeAndReadme()
	return dedupeSuggestions(root, s.out)
}

type suggester struct {
	root string
	lang string
	out  []CheckSuggestion
}

func (s *suggester) add(title, why core.Msg, c OracleCheck, setup string) {
	s.out = append(s.out, CheckSuggestion{Title: title.Render(s.lang), Why: why.Render(s.lang), TitleMsg: title, WhyMsg: why, Check: c, Setup: setup})
}

func (s *suggester) node() {
	b, err := os.ReadFile(filepath.Join(s.root, "package.json"))
	if err != nil {
		return
	}
	var pkg struct {
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal(b, &pkg)
	pm, setup := nodePackageManager(s.root)
	if len(pkg.Dependencies)+len(pkg.DevDependencies) == 0 {
		setup = ""
	}
	run := func(script string) string {
		if script == "test" && pm != "bun" {
			return pm + " test"
		}
		return pm + " run " + script
	}
	if script := pkg.Scripts["test"]; script != "" && !strings.Contains(script, "no test specified") {
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_script", "command", run("test")), cmdCheck("testes", "The tests pass", run("test")), setup)
	} else if hasFileMatching(s.root, `\.test\.(m?js|cjs)$`) {
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_node_test"), cmdCheck("testes", "The tests pass", "node --test"), setup)
	}
	if pkg.Scripts["build"] != "" {
		s.add(core.M("suggest.builds"), core.M("suggest.why_run_ok", "command", run("build")), cmdCheck("build", "The project builds", run("build")), setup)
	}
	if pkg.Scripts["lint"] != "" {
		s.add(core.M("suggest.lint"), core.M("suggest.why_run_ok", "command", run("lint")), cmdCheck("lint", "No lint errors", run("lint")), setup)
	}
	_, ts := pkg.DevDependencies["typescript"]
	if _, d := pkg.Dependencies["typescript"]; d {
		ts = true
	}
	if pkg.Scripts["typecheck"] != "" {
		s.add(core.M("suggest.types"), core.M("suggest.why_run", "command", run("typecheck")), cmdCheck("tipos", "No type errors", run("typecheck")), setup)
	} else if ts && hasPath(s.root, "tsconfig.json") {
		s.add(core.M("suggest.types"), core.M("suggest.why_tsc"), cmdCheck("tipos", "No type errors", "npx tsc --noEmit"), setup)
	}
	if port := nodePort(pkg.Dependencies, pkg.DevDependencies); port != "" {
		c := OracleCheck{ID: "pagina", Kind: "http", URL: "http://localhost:" + port + "/", Description: "The home page opens"}
		c.Expect.Status = 200
		s.add(core.M("suggest.home_opens"), core.M("suggest.why_page", "url", "http://localhost:"+port+"/"), c, "")
	}
}

func nodePackageManager(root string) (string, string) {
	switch {
	case hasPath(root, "pnpm-lock.yaml"):
		return "pnpm", "pnpm install --frozen-lockfile"
	case hasPath(root, "yarn.lock"):
		return "yarn", "yarn install --frozen-lockfile"
	case hasPath(root, "bun.lockb"), hasPath(root, "bun.lock"):
		return "bun", "bun install"
	case hasPath(root, "package-lock.json"):
		return "npm", "npm ci"
	}
	return "npm", "npm install"
}

func nodePort(deps ...map[string]string) string {
	has := func(name string) bool {
		for _, d := range deps {
			if d[name] != "" {
				return true
			}
		}
		return false
	}
	switch {
	case has("vite"):
		return "5173"
	case has("next"), has("react-scripts"), has("express"):
		return "3000"
	case has("@angular/core"):
		return "4200"
	}
	return ""
}

func (s *suggester) goProject() {
	if !hasPath(s.root, "go.mod") {
		return
	}
	s.add(core.M("suggest.builds"), core.M("suggest.why_run", "command", "go build ./..."), cmdCheck("build", "The project builds", "go build ./..."), "")
	s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "go test ./..."), cmdCheck("testes", "The tests pass", "go test ./..."), "")
	s.add(core.M("suggest.vet"), core.M("suggest.why_vet"), cmdCheck("vet", "No go vet problems", "go vet ./..."), "")
}

func (s *suggester) otherLanguages() {
	s.rust()
	s.python()
	s.dotnet()
	s.jvm()
	s.make()
}

func (s *suggester) rust() {
	if !hasPath(s.root, "Cargo.toml") {
		return
	}
	s.add(core.M("suggest.builds"), core.M("suggest.why_run", "command", "cargo build"), cmdCheck("build", "The project builds", "cargo build"), "")
	s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "cargo test"), cmdCheck("testes", "The tests pass", "cargo test"), "")
}

func (s *suggester) python() {
	if !hasPath(s.root, "pyproject.toml") && !hasPath(s.root, "requirements.txt") && !hasPath(s.root, "setup.py") {
		return
	}
	setup := ""
	if hasPath(s.root, "requirements.txt") {
		setup = "python -m pip install -r requirements.txt"
	}
	if hasPath(s.root, "tests") || hasPath(s.root, "pytest.ini") || hasFileMatching(s.root, `(^|/)test_[^/]+\.py$`) {
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "python -m pytest"), cmdCheck("testes", "The tests pass", "python -m pytest -q"), setup)
	}
	s.add(core.M("suggest.python_valid"), core.M("suggest.why_compileall"), cmdCheck("sintaxe", "Every Python file is valid", "python -m compileall -q ."), "")
}

func (s *suggester) dotnet() {
	if !hasFileMatching(s.root, `\.(sln|csproj)$`) {
		return
	}
	s.add(core.M("suggest.builds"), core.M("suggest.why_run", "command", "dotnet build"), cmdCheck("build", "The project builds", "dotnet build"), "")
	s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "dotnet test"), cmdCheck("testes", "The tests pass", "dotnet test"), "")
}

func (s *suggester) jvm() {
	if hasPath(s.root, "pom.xml") {
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "mvn -q test"), cmdCheck("testes", "The tests pass", "mvn -q test"), "")
	}
	if hasPath(s.root, "build.gradle") || hasPath(s.root, "build.gradle.kts") {
		g := "gradle"
		if hasPath(s.root, "gradlew") {
			g = "./gradlew"
		}
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", g+" test"), cmdCheck("testes", "The tests pass", g+" test"), "")
	}
}

func (s *suggester) make() {
	if b, err := os.ReadFile(filepath.Join(s.root, "Makefile")); err == nil && reMakeTest.Match(b) {
		s.add(core.M("suggest.tests_pass"), core.M("suggest.why_run", "command", "make test"), cmdCheck("testes", "The tests pass", "make test"), "")
	}
}

func (s *suggester) sizeAndReadme() {
	if ext := mainExtension(s.root); ext != "" {
		c := OracleCheck{ID: "tamanho", Kind: "maxLines", Glob: "**/*" + ext, Max: 400, Description: "No file is too large"}
		s.add(core.M("suggest.no_large_file"), core.M("suggest.why_size", "ext", ext), c, "")
	}
	if hasPath(s.root, "README.md") {
		c := OracleCheck{ID: "readme", Kind: "file", Path: "README.md", Description: "The README exists"}
		s.add(core.M("suggest.readme"), core.M("suggest.why_readme"), c, "")
	}
}

func dedupeSuggestions(root string, out []CheckSuggestion) []CheckSuggestion {
	have := map[string]bool{}
	if spec, err := LoadOracleSpec(root); err == nil {
		for _, c := range spec.Checks {
			have[c.Kind+"|"+c.Run+c.URL+c.Path+c.Glob] = true
		}
	}
	seen := map[string]bool{}
	res := []CheckSuggestion{}
	for _, s := range out {
		k := s.Check.Kind + "|" + s.Check.Run + s.Check.URL + s.Check.Path + s.Check.Glob
		if have[k] || seen[k] {
			continue
		}
		seen[k] = true
		res = append(res, s)
	}
	return res
}

// hasFileMatching walks a few levels of the project (skipping dependencies) for a file whose path matches.
func hasFileMatching(root, pattern string) bool {
	re := regexp.MustCompile(pattern)
	found := false
	_ = fsx.Walk(root, fsx.Project, func(p string, d os.DirEntry, err error) error {
		if found {
			return filepath.SkipAll
		}
		if err != nil {
			return nil
		}
		rel := filepath.ToSlash(core.RelTo(root, p))
		if d.IsDir() {
			if p != root && strings.Count(rel, "/") >= 3 {
				return filepath.SkipDir
			}
			return nil
		}
		if re.MatchString(rel) {
			found = true
		}
		return nil
	})
	return found
}

// mainExtension is the most common source-file extension in the project.
func mainExtension(root string) string {
	src := map[string]bool{".go": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true, ".py": true, ".rs": true, ".cs": true, ".java": true, ".kt": true, ".rb": true, ".php": true, ".swift": true, ".vue": true, ".svelte": true}
	count := map[string]int{}
	_ = fsx.Walk(root, fsx.Project, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if e := strings.ToLower(filepath.Ext(p)); src[e] {
			count[e]++
		}
		return nil
	})
	var exts []string
	for e := range count {
		exts = append(exts, e)
	}
	sort.Slice(exts, func(i, j int) bool {
		return count[exts[i]] > count[exts[j]] || count[exts[i]] == count[exts[j]] && exts[i] < exts[j]
	})
	if len(exts) == 0 {
		return ""
	}
	return exts[0]
}
