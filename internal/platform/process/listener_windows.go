//go:build windows

package process

// Who owns a listening port: the TCP tables of both address families name the owning process, and the
// process's membership in the job of the command it was started under decides whether it is ours.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	afInet             = 2
	afInet6            = 23
	tcpTableListener   = 3 // TCP_TABLE_OWNER_PID_LISTENER
	tcpRowSize         = 24
	tcp6RowSize        = 56
	tcpRowPortOffset   = 8
	tcpRowPIDOffset    = 20
	tcp6RowPortOffset  = 20
	tcp6RowPIDOffset   = 52
	errInsufficientBuf = 122
)

var (
	iphlpapi                = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = iphlpapi.NewProc("GetExtendedTcpTable")
	procIsProcessInJob      = kernel32.NewProc("IsProcessInJob")
)

// tcpTable returns the raw listener table of an address family.
func tcpTable(family uint32) ([]byte, error) {
	size := uint32(64 << 10)
	for range 4 {
		buf := make([]byte, size)
		r, _, _ := procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), tcpTableListener, 0)
		switch r {
		case 0:
			return buf, nil
		case errInsufficientBuf:
			continue
		default:
			return nil, fmt.Errorf("GetExtendedTcpTable: %d", r)
		}
	}
	return nil, errors.New("GetExtendedTcpTable: the table keeps growing")
}

// listenersIn extracts the pids listening on port from a raw table.
func listenersIn(table []byte, port, rowSize, portAt, pidAt int) []int {
	if len(table) < 4 {
		return nil
	}
	var out []int
	count := int(binary.LittleEndian.Uint32(table))
	for i := range count {
		row := 4 + i*rowSize
		if row+rowSize > len(table) {
			break
		}
		if int(binary.BigEndian.Uint16(table[row+portAt:])) == port {
			out = append(out, int(binary.LittleEndian.Uint32(table[row+pidAt:])))
		}
	}
	return out
}

// ListenerPIDs lists the processes listening on a TCP port, IPv4 and IPv6.
func ListenerPIDs(port int) ([]int, error) {
	v4, err := tcpTable(afInet)
	if err != nil {
		return nil, err
	}
	v6, err := tcpTable(afInet6)
	if err != nil {
		return nil, err
	}
	return append(listenersIn(v4, port, tcpRowSize, tcpRowPortOffset, tcpRowPIDOffset),
		listenersIn(v6, port, tcp6RowSize, tcp6RowPortOffset, tcp6RowPIDOffset)...), nil
}

// inJob reports whether pid belongs to job.
func inJob(job windows.Handle, pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var member int32
	r, _, _ := procIsProcessInJob.Call(uintptr(h), uintptr(job), uintptr(unsafe.Pointer(&member)))
	return r != 0 && member != 0
}

// OwnsListener reports whether something listens on port and every listener belongs to the process
// tree of cmd, which AttachJob must have put in a job.
func OwnsListener(cmd *exec.Cmd, port int) (bool, error) {
	procJobsMu.Lock()
	job, ok := procJobs[cmd]
	procJobsMu.Unlock()
	if !ok {
		return false, errors.New("the command has no job")
	}
	pids, err := ListenerPIDs(port)
	if err != nil {
		return false, err
	}
	for _, pid := range pids {
		if !inJob(job, pid) {
			return false, nil
		}
	}
	return len(pids) > 0, nil
}
