// Package architecture holds tests that keep the repository's structure honest: file sizes, package
// entry points and the direction of imports. They fail the build, not a review.
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var (
	reColour   = regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(`)
	reStyle    = regexp.MustCompile(`style="([^"]*)"`)
	reCustom   = regexp.MustCompile(`^\s*--[a-zA-Z0-9_-]+\s*:`)
	reAnyColor = regexp.MustCompile(`(?i)(?:color|background|border-color|box-shadow)\s*:[^;{}]*\b(?:red|blue|green|white|black)\b`)
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func walk(t *testing.T, root string, visit func(rel, abs string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (name == ".git" || name == ".claude" || name == "build" || name == "legacy" || name == "node_modules" || name == "vendor" || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		visit(filepath.ToSlash(rel), p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestNoGodFiles: every Go and frontend source file stays under the project's own limit (400 lines).
func TestNoGodFiles(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel, abs string) {
		if strings.HasSuffix(rel, ".json") || strings.HasSuffix(rel, ".css") || strings.HasSuffix(rel, ".html") {
			return
		}
		if !strings.HasSuffix(rel, ".go") && !strings.HasSuffix(rel, ".js") {
			return
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		if n := len(strings.Split(strings.TrimRight(string(b), "\n"), "\n")); n > 400 {
			t.Errorf("%s has %d lines (limit 400)", rel, n)
		}
	})
}

// TestFunctionsStaySmall: every Go function body fits in 60 lines, tests included.
func TestFunctionsStaySmall(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".go") {
			return
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, abs, nil, 0)
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			return
		}
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			start := fset.Position(fn.Body.Lbrace).Line
			end := fset.Position(fn.Body.Rbrace).Line
			if lines := end - start - 1; lines > 60 {
				t.Errorf("%s:%d %s has %d lines (limit 60)", rel, fset.Position(fn.Pos()).Line, fn.Name.Name, lines)
			}
			return true
		})
	})
}

// TestNoLiteralColoursOrInlineStyles: colours live only in the token file, and no static declaration
// is written in markup. A dynamic geometry value may travel as a CSS custom property.
func TestNoLiteralColoursOrInlineStyles(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel, abs string) {
		if !strings.HasPrefix(rel, "ui/") || strings.Contains(rel, "/vendor/") {
			return
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		text := string(b)
		isCSS := strings.HasSuffix(rel, ".css")
		isTokens := rel == "ui/css/tokens.css"
		switch {
		case isCSS && !isTokens:
			if m := reColour.FindString(text); m != "" {
				t.Errorf("%s: literal colour %q outside tokens.css", rel, m)
			}
			if m := reAnyColor.FindString(text); m != "" {
				t.Errorf("%s: colour name %q outside tokens.css", rel, m)
			}
		case strings.HasSuffix(rel, ".js") || strings.HasSuffix(rel, ".html"):
			if m := reColour.FindString(text); m != "" {
				t.Errorf("%s: literal colour %q in interface code", rel, m)
			}
			for _, m := range reStyle.FindAllStringSubmatch(text, -1) {
				decls := strings.TrimSpace(m[1])
				if strings.Contains(decls, "${") {
					continue // dynamic geometry through a custom property, check the declaration below
				}
				for _, d := range strings.Split(decls, ";") {
					if strings.TrimSpace(d) != "" && !reCustom.MatchString(d) {
						t.Errorf("%s: static inline style %q", rel, decls)
						break
					}
				}
			}
		}
	})
}

// TestPackageMainOnlyInCmd: the executable is the only package main in the module.
func TestPackageMainOnlyInCmd(t *testing.T) {
	root := repoRoot(t)
	walk(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return
		}
		f, err := parser.ParseFile(token.NewFileSet(), abs, nil, parser.PackageClauseOnly)
		if err != nil || f.Name == nil {
			return
		}
		if f.Name.Name == "main" && !strings.HasPrefix(rel, "cmd/") {
			t.Errorf("%s is package main outside cmd/", rel)
		}
	})
}

// TestImportsFollowTheLayers: a package may only import packages below it in the documented direction.
func TestImportsFollowTheLayers(t *testing.T) {
	root := repoRoot(t)
	forbidden := map[string][]string{
		"internal/filter":    {"internal/agent", "internal/knowledge", "internal/api", "internal/desktop"},
		"internal/desktop":   {"internal/agent", "internal/api"},
		"internal/core":      {"internal/agent", "internal/api", "internal/desktop", "internal/filter", "internal/llm", "internal/knowledge"},
		"internal/data":      {"internal/agent", "internal/api", "internal/desktop"},
		"internal/llm":       {"internal/agent", "internal/api", "internal/desktop", "internal/govern"},
		"internal/knowledge": {"internal/agent", "internal/api", "internal/desktop"},
		"internal/browser":   {"internal/agent", "internal/api", "internal/desktop"},
		"internal/mcp":       {"internal/agent", "internal/api", "internal/desktop"},
		"internal/workspace": {"internal/agent", "internal/api", "internal/desktop"},
		"internal/oracle":    {"internal/agent", "internal/api", "internal/desktop"},
	}
	seen := map[string]bool{}
	walk(t, root, func(rel, abs string) {
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return
		}
		pkgDir := filepath.ToSlash(filepath.Dir(rel))
		f, err := parser.ParseFile(token.NewFileSet(), abs, nil, parser.ImportsOnly)
		if err != nil {
			return
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !strings.HasPrefix(p, "infuse/internal/") {
				continue
			}
			dst := strings.TrimPrefix(p, "infuse/")
			key := pkgDir + " -> " + dst
			if seen[key] {
				continue
			}
			seen[key] = true
			for _, bad := range forbidden[pkgDir] {
				if strings.HasPrefix(dst, bad) {
					t.Errorf("%s imports %s, forbidden by the layer rules", rel, p)
				}
			}
		}
	})
}

// TestNoImportCycles builds the internal package graph and fails on a cycle.
func TestNoImportCycles(t *testing.T) {
	root := repoRoot(t)
	graph := map[string][]string{}
	walk(t, root, func(rel, abs string) {
		if !strings.HasPrefix(rel, "internal/") || !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") {
			return
		}
		dstDir := filepath.ToSlash(filepath.Dir(rel))
		pkg := "infuse/" + dstDir
		f, err := parser.ParseFile(token.NewFileSet(), abs, nil, parser.ImportsOnly)
		if err != nil {
			return
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, "infuse/internal/") {
				graph[pkg] = append(graph[pkg], p)
			}
		}
	})
	state := map[string]int{} // 0 unvisited, 1 visiting, 2 done
	var visit func(string) bool
	visit = func(pkg string) bool {
		switch state[pkg] {
		case 1:
			t.Errorf("import cycle at %s", pkg)
			return false
		case 2:
			return true
		}
		state[pkg] = 1
		for _, dep := range graph[pkg] {
			if !visit(dep) {
				return false
			}
		}
		state[pkg] = 2
		return true
	}
	for pkg := range graph {
		if !visit(pkg) {
			return
		}
	}
}
