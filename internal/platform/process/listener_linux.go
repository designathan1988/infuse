//go:build linux

package process

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const tcpListenState = "0A"

// listenerInodes returns the socket inodes listening on port, from the kernel's TCP tables.
func listenerInodes(port int) map[string]bool {
	out := map[string]bool{}
	for _, table := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		b, err := os.ReadFile(table)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			f := strings.Fields(line)
			if len(f) < 10 || f[3] != tcpListenState {
				continue
			}
			_, hexPort, ok := strings.Cut(f[1], ":")
			if p, err := strconv.ParseInt(hexPort, 16, 32); ok && err == nil && int(p) == port {
				out[f[9]] = true
			}
		}
	}
	return out
}

// ListenerPIDs lists the processes holding a listening socket on a TCP port.
func ListenerPIDs(port int) ([]int, error) {
	inodes := listenerInodes(port)
	if len(inodes) == 0 {
		return nil, nil
	}
	procs, err := os.ReadDir("/proc")
	if err != nil {
		return nil, err
	}
	var out []int
	for _, p := range procs {
		pid, err := strconv.Atoi(p.Name())
		if err != nil {
			continue
		}
		fds, _ := os.ReadDir(filepath.Join("/proc", p.Name(), "fd"))
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", p.Name(), "fd", fd.Name()))
			if err == nil && inodes[strings.TrimSuffix(strings.TrimPrefix(link, "socket:["), "]")] {
				out = append(out, pid)
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("port %d has a listener this user cannot attribute", port)
	}
	return out, nil
}
