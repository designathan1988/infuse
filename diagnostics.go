package main

// Diagnostics after each write: the file is checked with the project's own tools (the role a language
// server plays in an editor), and problems go back to the agent with the write's result, so a broken
// file is noticed at once instead of at the end of the turn. No server to install: the checks are the
// compiler or checker the project already uses, limited to a few seconds.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// fileDiagnostics returns the problems found in abs ("" when it is fine or no checker applies).
func fileDiagnostics(root, abs string) string {
	ext := strings.ToLower(filepath.Ext(abs))
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	run := func(dir, name string, args ...string) (string, bool) {
		exe, err := exec.LookPath(name)
		if err != nil {
			return "", true // checker not installed: nothing to say
		}
		cmd := exec.CommandContext(ctx, exe, args...)
		noConsoleWindow(cmd)
		cmd.Dir = dir
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		err = cmd.Run()
		return strings.TrimSpace(out.String()), err == nil
	}
	rel := filepath.ToSlash(relTo(root, abs))
	switch ext {
	case ".json":
		b, err := os.ReadFile(abs)
		if err == nil && !json.Valid(b) {
			var v any
			err := json.Unmarshal(b, &v)
			return rel + ": JSON inválido: " + err.Error()
		}
	case ".js", ".mjs", ".cjs":
		if out, ok := run(filepath.Dir(abs), "node", "--check", abs); !ok && out != "" {
			return truncate(out, 1500)
		}
	case ".py":
		if out, ok := run(filepath.Dir(abs), "python", "-m", "py_compile", abs); !ok && out != "" {
			return truncate(out, 1500)
		}
	case ".go":
		mod := findUp(filepath.Dir(abs), "go.mod")
		if mod == "" {
			return ""
		}
		if out, ok := run(filepath.Dir(abs), "gofmt", "-e", "-l", abs); !ok && out != "" {
			return truncate(out, 1500)
		}
		if out, ok := run(filepath.Dir(abs), "go", "vet", "."); !ok && out != "" {
			return truncate(out, 2000)
		}
	case ".ts", ".tsx", ".mts", ".cts":
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
		out, ok := run(dir, tsc, "--noEmit", "--pretty", "false", "-p", ".")
		if ok || out == "" {
			return ""
		}
		// only the lines about this file: the rest of the project is not this edit's concern
		var mine []string
		base := filepath.ToSlash(relTo(dir, abs))
		for _, l := range strings.Split(out, "\n") {
			if strings.HasPrefix(filepath.ToSlash(l), base+"(") {
				mine = append(mine, l)
			}
		}
		return truncate(strings.Join(mine, "\n"), 2000)
	}
	return ""
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
