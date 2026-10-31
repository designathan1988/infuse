//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

func Detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func ProcessAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

func NoConsoleWindow(cmd *exec.Cmd) {}

func AttachParentConsole() {}

// procGroup puts the command in its own process group, so killTree reaches what it started.
func ProcGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killTree ends the process group.
func KillTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) != nil {
		_ = cmd.Process.Kill()
	}
}

// attachJob and releaseJob: the process group set by procGroup does this job on Unix.
func AttachJob(cmd *exec.Cmd)  {}
func ReleaseJob(cmd *exec.Cmd) {}
