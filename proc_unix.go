//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func noConsoleWindow(cmd *exec.Cmd) {}

func attachParentConsole() {}
