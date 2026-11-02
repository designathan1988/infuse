//go:build !windows && !linux

package process

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// ListenerPIDs lists the processes listening on a TCP port. lsof exits 1 when nothing listens, with no
// output; a missing lsof is an error, so the caller refuses instead of trusting the port.
func ListenerPIDs(port int) ([]int, error) {
	path, err := exec.LookPath("lsof")
	if err != nil {
		return nil, fmt.Errorf("lsof is required to verify a listener: %w", err)
	}
	out, _ := exec.Command(path, "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t").Output()
	var pids []int
	for _, f := range strings.Fields(string(out)) {
		if pid, err := strconv.Atoi(f); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids, nil
}
