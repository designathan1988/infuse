package main

// infuse — one executable, three roles:
//   infuse(.exe) double-clicked / no args   → the application window
//   bash(.exe) (this binary copied by install) → the interposed shell
//   infuse <command>                         → CLI
//
// Everything the agent reads arrives infused with the project's knowledge; everything it writes
// or runs is transformed by that knowledge before it touches disk. See SPEC.md.

import (
	"fmt"
	"infuse/internal/api"
	"infuse/internal/core"
	"infuse/internal/desktop"
	"infuse/internal/filter"
	"infuse/internal/knowledge"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var version = core.Version

// shellPolicy supplies the filter with the project-specific protection (instructions and references)
// without making the filter depend on the knowledge layer.
func shellPolicy(root string, protected []string) filter.Policy {
	return filter.Policy{
		Protected:        protected,
		References:       knowledge.LoadProjectRules(root).Reference,
		InstructionFiles: knowledge.InstructionNames,
		Memory:           []string{knowledge.MemoryRel},
	}
}

func main() {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"))
	if name == "bash" || name == "sh" {
		// The binary is a GUI-subsystem program (no console window of its own). Invoked as the shell it
		// inherits the agent's pipes; from an interactive terminal it borrows that terminal's console.
		process.AttachParentConsole()
		os.Exit(filter.RunShell(os.Args[1:], shellPolicy))
	}
	if len(os.Args) < 2 {
		api.Start(true)
		return
	}
	process.AttachParentConsole()
	os.Exit(runCommand(os.Args[1], os.Args[2:]))
}

// runCommand dispatches one CLI command and returns the process exit code.
func runCommand(command string, args []string) int {
	cwd, _ := os.Getwd()
	root := core.FindRoot(cwd)
	switch command {
	case "shell":
		return filter.RunShell(args, shellPolicy)
	case "ui", "panel", "window":
		api.Start(true)
	case "serve":
		if len(args) > 0 {
			os.Setenv("INFUSE_PORT", args[0])
		}
		api.Start(false)
	case "claude", "codex", "gemini":
		return launchAgent(command, args)
	case "install":
		return installCommand()
	case "uninstall":
		uninstallCommand()
	case "status":
		statusCommand(root)
	case "watch":
		watchCommand(root, args)
	case "map":
		mapCommand(root)
	case "journal":
		journalCommand(root, args)
	case "lessons":
		lessonsCommand(root)
	case "rules":
		rulesCommand(root)
	case "version", "--version", "-v":
		fmt.Println("infuse", version)
	default:
		fmt.Println("usage: infuse [ui | claude | codex | gemini | install | uninstall | status | map | journal [n] | lessons | rules | shell -c <cmd> | watch <root> | version]")
		return 1
	}
	return 0
}

// launchAgent starts an external coding agent with its shell pointing at the interposed shim: every
// command it runs passes through the filter, without changing the user's global settings.
func launchAgent(kind string, args []string) int {
	exe := desktop.CLIPath(kind)
	if exe == "" {
		fmt.Printf("[infuse] %s was not found on this machine; install the CLI or use the bridge provider.\n", kind)
		return 1
	}
	if err := filter.EnsureShim(); err != nil {
		fmt.Println("[infuse] cannot install the interposed shell:", err)
		return 1
	}
	shim := filter.ShimPath()
	cmd := exec.Command(exe, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Dir, _ = os.Getwd()
	env := setEnv(os.Environ(), "CLAUDE_CODE_GIT_BASH_PATH", shim)
	env = setEnv(env, "CLAUDE_CODE_SHELL", shim)
	env = setEnv(env, "SHELL", shim)
	env = setEnv(env, "PATH", filepath.Dir(shim)+string(os.PathListSeparator)+os.Getenv("PATH"))
	env = setEnv(env, "INFUSE_LAUNCHED", "1")
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "[infuse] cannot start", kind+":", err)
		return 1
	}
	return 0
}

// setEnv replaces a variable (case-insensitively, as Windows does) instead of appending a duplicate.
func setEnv(env []string, key, value string) []string {
	out := env[:0]
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !strings.EqualFold(k, key) {
			out = append(out, e)
		}
	}
	return append(out, key+"="+value)
}

// installCommand installs the interposed shell; 1 when installation failed.
func installCommand() int {
	res := filter.Install()
	if res.Error != "" {
		fmt.Println("[infuse] ERROR:", res.Error)
		return 1
	}
	fmt.Printf("[infuse] interposed shell installed: %s\n[infuse] real bash: %s\n[infuse] Claude Code pointed at it via %s (%s)\n[infuse] restart Claude Code. From then on every Bash tool command passes through the filter.\n", res.Shim, res.RealBash, res.SettingsFile, res.EnvKey)
	return 0
}

func uninstallCommand() {
	res := filter.Uninstall()
	fmt.Println("[infuse] removed. Claude Code is back to its own shell detection (restart it).")
	if res.Error != "" {
		fmt.Println("[infuse] warning:", res.Error)
	}
}

func statusCommand(root string) {
	filter.PrintJSON(filter.GetStatus())
	pid, alive := filter.WatcherAlive(root)
	fmt.Printf("project root here: %s\nwatcher: pid %d alive=%v\n", root, pid, alive)
}

func watchCommand(root string, args []string) {
	if len(args) > 0 {
		root = args[0]
	}
	idle := 45 * time.Minute
	if v, err := strconv.Atoi(os.Getenv("INFUSE_WATCHER_IDLE_MIN")); err == nil && v > 0 {
		idle = time.Duration(v) * time.Minute
	}
	filter.RunWatcher(root, idle)
}

func mapCommand(root string) {
	m := projectmap.BuildMap(root)
	projectmap.WriteMap(root, m)
	fmt.Printf("[infuse] map: %d files, %d dirs, %d hotspot(s) → %s\n", len(m.Files), len(m.Dirs), len(m.Hotspots), projectmap.MapFile(root))
	for _, h := range m.Hotspots {
		fmt.Printf("  ⚠ %s: %d lines\n", h.Path, h.Lines)
	}
}

func journalCommand(root string, args []string) {
	n := 20
	if len(args) > 0 {
		n, _ = strconv.Atoi(args[0])
	}
	for _, e := range tailJSONL(filepath.Join(root, core.InfuseDirName, "journal.jsonl"), n) {
		cmdText, _ := e["command"].(string)
		p, _ := e["path"].(string)
		if p == "" {
			p = core.Truncate(cmdText, 80)
		}
		fmt.Printf("%s %-9s %s %s\n", tsOf(e), e["kind"], p, e["detail"])
	}
}

func lessonsCommand(root string) {
	for _, e := range tailJSONL(filepath.Join(root, core.InfuseDirName, "lessons.jsonl"), 50) {
		fmt.Printf("%s %s: %s\n", tsOf(e), e["vice"], e["detail"])
	}
}

func rulesCommand(root string) {
	set := filter.LoadRules(root)
	fmt.Printf("[infuse] rule files in effect: %s\n", strings.Join(set.Sources, ", "))
	for i, r := range set.Rules {
		fmt.Printf("  %d. [%s] %s match=/%s/\n", i+1, r.When, r.ID, r.Match)
	}
}

func tailJSONL(file string, n int) []map[string]any {
	all := core.ReadJSONL(file)
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}

func tsOf(e map[string]any) string {
	if f, ok := e["ts"].(float64); ok {
		return time.UnixMilli(int64(f)).Format("2006-01-02 15:04:05")
	}
	return ""
}
