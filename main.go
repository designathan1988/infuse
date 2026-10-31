package main

// infuse — one executable, three roles:
//   infuse(.exe) double-clicked / no args   → the panel (local web UI)
//   bash(.exe) (this binary copied by install) → the interposed shell
//   infuse <command>                         → CLI
//
// Everything the agent reads arrives infused with the project's knowledge; everything it writes
// or runs is transformed by that knowledge before it touches disk. See SPEC.md.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const version = "0.5.0"

func main() {
	name := strings.ToLower(strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"))
	if name == "bash" || name == "sh" {
		// The binary is a GUI-subsystem program (no console window of its own). Invoked as the shell it
		// inherits the agent's pipes; from an interactive terminal it borrows that terminal's console.
		attachParentConsole()
		os.Exit(RunShell(os.Args[1:]))
	}
	if len(os.Args) < 2 {
		StartUI(true)
		return
	}
	attachParentConsole()
	args := os.Args[2:]
	cwd, _ := os.Getwd()
	root := FindRoot(cwd)
	switch os.Args[1] {
	case "shell":
		os.Exit(RunShell(args))
	case "ui", "panel", "window":
		StartUI(true)
	case "serve":
		if len(args) > 0 {
			os.Setenv("INFUSE_PORT", args[0])
		}
		StartUI(false)
	case "install":
		res := Install()
		if res.Error != "" {
			fmt.Println("[infuse] ERROR:", res.Error)
			os.Exit(1)
		}
		fmt.Printf("[infuse] interposed shell installed: %s\n[infuse] real bash: %s\n[infuse] Claude Code pointed at it via %s (%s)\n[infuse] restart Claude Code. From then on every Bash tool command passes through the filter.\n", res.Shim, res.RealBash, res.SettingsFile, res.EnvKey)
	case "uninstall":
		res := Uninstall()
		fmt.Println("[infuse] removed. Claude Code is back to its own shell detection (restart it).")
		if res.Error != "" {
			fmt.Println("[infuse] warning:", res.Error)
		}
	case "status":
		printJSON(GetStatus())
		pid, alive := watcherAlive(root)
		fmt.Printf("project root here: %s\nwatcher: pid %d alive=%v\n", root, pid, alive)
	case "watch":
		if len(args) > 0 {
			root = args[0]
		}
		idle := 45 * time.Minute
		if v, err := strconv.Atoi(os.Getenv("INFUSE_WATCHER_IDLE_MIN")); err == nil && v > 0 {
			idle = time.Duration(v) * time.Minute
		}
		RunWatcher(root, idle)
	case "map":
		m := BuildMap(root)
		WriteMap(root, m)
		fmt.Printf("[infuse] map: %d files, %d dirs, %d hotspot(s) → %s\n", len(m.Files), len(m.Dirs), len(m.Hotspots), mapFile(root))
		for _, h := range m.Hotspots {
			fmt.Printf("  ⚠ %s: %d lines\n", h.Path, h.Lines)
		}
	case "journal":
		n := 20
		if len(args) > 0 {
			n, _ = strconv.Atoi(args[0])
		}
		for _, e := range tailJSONL(filepath.Join(root, infuseDirName, "journal.jsonl"), n) {
			cmdText, _ := e["command"].(string)
			p, _ := e["path"].(string)
			if p == "" {
				p = truncate(cmdText, 80)
			}
			fmt.Printf("%s %-9s %s %s\n", tsOf(e), e["kind"], p, e["detail"])
		}
	case "lessons":
		for _, e := range tailJSONL(filepath.Join(root, infuseDirName, "lessons.jsonl"), 50) {
			fmt.Printf("%s %s: %s\n", tsOf(e), e["vice"], e["detail"])
		}
	case "rules":
		set := LoadRules(root)
		fmt.Printf("[infuse] rule files in effect: %s\n", strings.Join(set.Sources, ", "))
		for i, r := range set.Rules {
			fmt.Printf("  %d. [%s] %s match=/%s/\n", i+1, r.When, r.ID, r.Match)
		}
	case "version", "--version", "-v":
		fmt.Println("infuse", version)
	default:
		fmt.Println("usage: infuse [ui | install | uninstall | status | map | journal [n] | lessons | rules | shell -c <cmd> | watch <root> | version]")
		os.Exit(1)
	}
}

func tailJSONL(file string, n int) []map[string]any {
	all := readJSONL(file)
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
