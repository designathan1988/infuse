package filter

// Diagnostics after each write: the file is checked with the project's own tools (the role a language
// server plays in an editor), and problems go back to the agent with the write's result, so a broken
// file is noticed at once instead of at the end of the turn. No server to install: the checks are the
// compiler or checker the project already uses, limited to a few seconds. These checkers read and may
// load code the agent just wrote (a project's tsc, a Python module beside the file), so they run
// contained, like the agent's own shell.

import (
	"bytes"
	"context"
	"encoding/json"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/platform/process"
	"infuse/internal/sandbox"
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
		return nodeDiagnostics(ctx, root, abs)
	case ".py":
		return pythonDiagnostics(ctx, root, abs)
	case ".go":
		return goDiagnostics(ctx, root, abs)
	case ".ts", ".tsx", ".mts", ".cts":
		return typescriptDiagnostics(ctx, root, abs)
	}
	return ""
}

// notRunExit is the sandbox's "could not be contained": the checker never ran, so it found nothing.
const notRunExit = 126

// runChecker runs a project checker contained to root, reporting its trimmed output and whether it
// succeeded.
func runChecker(ctx context.Context, root, dir, name string, args ...string) (string, bool) {
	exe, err := exec.LookPath(name)
	if err != nil {
		return "", true // checker not installed: nothing to say
	}
	argv := sandbox.Wrap(append([]string{exe}, args...), root, ContainedRecords(config.EffectiveSettings(root).ProtectedPaths), config.EffectiveSettings(root).ContainProcesses)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	if core.ExitCode(err) == notRunExit {
		return "", true
	}
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
func nodeDiagnostics(ctx context.Context, root, abs string) string {
	if out, ok := runChecker(ctx, root, filepath.Dir(abs), "node", "--check", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	return ""
}

// pythonDiagnostics checks Python with py_compile. -I keeps the file's folder off the module path: a
// py_compile.py planted beside the file must not be what runs.
func pythonDiagnostics(ctx context.Context, root, abs string) string {
	if out, ok := runChecker(ctx, root, filepath.Dir(abs), "python", "-I", "-m", "py_compile", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	return ""
}

// goDiagnostics checks Go with gofmt and go vet, inside a module.
func goDiagnostics(ctx context.Context, root, abs string) string {
	if findUp(filepath.Dir(abs), "go.mod") == "" {
		return ""
	}
	if out, ok := runChecker(ctx, root, filepath.Dir(abs), "gofmt", "-e", "-l", abs); !ok && out != "" {
		return core.Truncate(out, 1500)
	}
	if out, ok := runChecker(ctx, root, filepath.Dir(abs), "go", "vet", "."); !ok && out != "" {
		return core.Truncate(out, 2000)
	}
	return ""
}

// typescriptDiagnostics checks TypeScript with the project's own tsc.
func typescriptDiagnostics(ctx context.Context, root, abs string) string {
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
	out, ok := runChecker(ctx, root, dir, tsc, "--noEmit", "--pretty", "false", "-p", ".")
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
