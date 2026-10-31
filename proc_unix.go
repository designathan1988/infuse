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

// procGroup puts the command in its own process group, so killTree reaches what it started.
func procGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killTree ends the process group.
func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) != nil {
		_ = cmd.Process.Kill()
	}
}

// attachJob and releaseJob: the process group set by procGroup does this job on Unix.
func attachJob(cmd *exec.Cmd)  {}
func releaseJob(cmd *exec.Cmd) {}
