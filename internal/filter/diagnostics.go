package filter

// Diagnostics after each write: the file is checked with the project's own tools (the role a language
// server plays in an editor), and problems go back to the agent with the write's result, so a broken
// file is noticed at once instead of at the end of the turn. No server to install: the checks are the
// compiler or checker the project already uses, limited to a few seconds.

import (
	"bytes"
	"context"
	"encoding/json"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fileDiagnostics returns the problems found in abs ("" when it is fine or no checker applies).
func FileDiagnostics(root, abs string) string {
	ext := strings.ToLower(filepath.Ext(abs))
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	rel := filepath.ToSlash(core.RelTo(root, abs))
	switch ext {
	case ".json":
		return jsonDiagnostics(abs, rel)
	case ".js", ".mjs", ".cjs":
		return nodeDiagnostics(ctx, abs)
	case ".py":
		return pythonDiagnostics(ctx, abs)
	case ".go":
		return goDiagnostics(ctx, abs)
	case ".ts", ".tsx", ".mts", ".cts":
		return typescriptDiagnostics(ctx, abs)
	}
	return ""
}

// runChecker runs a project checker, reporting its trimmed output and whether it succeeded.
func runChecker(ctx context.Context, dir, name string, args ...string) (string, bool) {
	exe, err := exec.LookPath(name)
	if err != nil {
		return "", true // checker not installed: nothing to say
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return strings.TrimSpace(out.String()), err == nil
}

// jsonDiagnostics checks that the file is valid JSON.
func jsonDiagnostics(abs, rel string) string {
	b, err := os.ReadFile(abs)
	if err == nil && !json.Valid(b) {
		var v any
		err := json.Unmarshal(b, &v)
		return rel + ": invalid JSON: " + err.Error()
	}
	return ""
}

// nodeDiagnostics checks JavaScript with node --check.
func nodeDiagnostics(ctx context.Context, abs string) string {
	if out, ok := runChecker(ctx, filepath.Dir(abs), "node", "--check", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	return ""
}

// pythonDiagnostics checks Python with py_compile.
func pythonDiagnostics(ctx context.Context, abs string) string {
	if out, ok := runChecker(ctx, filepath.Dir(abs), "python", "-m", "py_compile", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	return ""
}

// goDiagnostics checks Go with gofmt and go vet, inside a module.
func goDiagnostics(ctx context.Context, abs string) string {
	if findUp(filepath.Dir(abs), "go.mod") == "" {
		return ""
	}
	if out, ok := runChecker(ctx, filepath.Dir(abs), "gofmt", "-e", "-l", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	if out, ok := runChecker(ctx, filepath.Dir(abs), "go", "vet", "."); !ok && out != "" {
		return core.Truncate(out, 2000)
	}
	return ""
}

// typescriptDiagnostics checks TypeScript with the project's own tsc.
func typescriptDiagnostics(ctx context.Context, abs string) string {
	dir := findUp(filepath.Dir(abs), "tsconfig.json")
	if dir == "" {
		return ""
	}
	tsc := filepath.Join(dir, "node_modules", ".bin", "tsc")
	if _, err := os.Stat(tsc); err != nil {
		if _, err := os.Stat(tsc + ".cmd"); err != nil {
			return "" // TypeScript not installed in the project
		}
		tsc += ".cmd"
	}
	out, ok := runChecker(ctx, dir, tsc, "--noEmit", "--pretty", "false", "-p", ".")
	if ok || out == "" {
		return ""
	}
	return typeScriptErrorsFor(dir, abs, out)
}

// typeScriptErrorsFor keeps only the checker's lines about this file: the rest of the project is not
// this edit's concern.
func typeScriptErrorsFor(dir, abs, out string) string {
	var mine []string
	base := filepath.ToSlash(core.RelTo(dir, abs))
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(filepath.ToSlash(l), base+"(") {
			mine = append(mine, l)
		}
	}
	return core.Truncate(strings.Join(mine, "\n"), 2000)
}

// findUp returns the nearest directory from dir upward that contains name.
func findUp(dir, name string) string {
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}
