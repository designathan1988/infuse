package main

import (
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/mountwin"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// mountCommand exposes a project through WinFsp. Writes pass the base and protected-path policy
// before they reach disk; the process keeps the mount alive until Ctrl+C or --timeout.
func mountCommand(args []string) int {
	var positional []string
	timeout := time.Duration(0)
	for i := 0; i < len(args); i++ {
		if args[i] == "--timeout" && i+1 < len(args) {
			seconds, err := strconv.Atoi(args[i+1])
			if err != nil || seconds < 0 {
				fmt.Fprintln(os.Stderr, "usage: infuse mount <drive|dir> [project] [--timeout seconds]")
				return 2
			}
			timeout = time.Duration(seconds) * time.Second
			i++
			continue
		}
		positional = append(positional, args[i])
	}
	if len(positional) < 1 {
		fmt.Fprintln(os.Stderr, "usage: infuse mount <drive|dir> [project] [--timeout seconds]")
		return 2
	}
	cwd, _ := os.Getwd()
	root := core.FindRoot(cwd)
	if len(positional) > 1 {
		root = positional[1]
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] mount:", err)
		return 1
	}
	protected := config.EffectiveSettings(abs).ProtectedPaths
	if err := mountwin.Run(positional[0], abs, protected, timeout); err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] mount:", err)
		return 1
	}
	return 0
}
