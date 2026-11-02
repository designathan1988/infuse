//go:build !windows

package process

import (
	"os/exec"
	"syscall"
)

// OwnsListener reports whether something listens on port and every listener belongs to the process
// group of cmd, which ProcGroup must have created.
func OwnsListener(cmd *exec.Cmd, port int) (bool, error) {
	pids, err := ListenerPIDs(port)
	if err != nil {
		return false, err
	}
	for _, pid := range pids {
		if pgid, err := syscall.Getpgid(pid); err != nil || pgid != cmd.Process.Pid {
			return false, nil
		}
	}
	return len(pids) > 0, nil
}
