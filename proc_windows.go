//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procAttachConsole    = kernel32.NewProc("AttachConsole")
)

// noConsoleWindow keeps console children (bash, git) from opening a console window of their own when
// this process has none: the binary is a GUI-subsystem program (-H windowsgui).
func noConsoleWindow(cmd *exec.Cmd) {
	if h, _, _ := procGetConsoleWindow.Call(); h != 0 {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// attachParentConsole: a GUI-subsystem binary gets no console. When run from a terminal (CLI use, or as
// bash.exe in an interactive shell), borrow the parent's console so output appears and children inherit
// it. Handles already inherited (pipes from an agent's Bash tool) are left untouched.
func attachParentConsole() {
	if r, _, _ := procAttachConsole.Call(^uintptr(0)); r == 0 { // ATTACH_PARENT_PROCESS
		return
	}
	valid := func(h windows.Handle) bool {
		if h == 0 || h == windows.InvalidHandle {
			return false
		}
		t, err := windows.GetFileType(h)
		return err == nil && t != windows.FILE_TYPE_UNKNOWN
	}
	if !valid(windows.Handle(os.Stdout.Fd())) {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stdout = f
		}
	}
	if !valid(windows.Handle(os.Stderr.Fd())) {
		if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
			os.Stderr = f
		}
	}
	if !valid(windows.Handle(os.Stdin.Fd())) {
		if f, err := os.OpenFile("CONIN$", os.O_RDWR, 0); err == nil {
			os.Stdin = f
		}
	}
}

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

// Background commands run inside a Job Object: every process they start belongs to it, MSYS ones
// included (their Windows parent links do not form a tree taskkill can follow), and ending the job ends
// all of them. The job is also closed — and its processes ended — if infuse itself goes away.
var (
	procJobsMu sync.Mutex
	procJobs   = map[*exec.Cmd]windows.Handle{}
)

// procGroup is applied before Start; the job is attached right after (attachJob).
func procGroup(cmd *exec.Cmd) {}

// attachJob puts a started process in a new kill-on-close job.
func attachJob(cmd *exec.Cmd) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return
	}
	defer windows.CloseHandle(h)
	if windows.AssignProcessToJobObject(job, h) != nil {
		windows.CloseHandle(job)
		return
	}
	procJobsMu.Lock()
	procJobs[cmd] = job
	procJobsMu.Unlock()
}

// killTree ends a process and every process it started: its job, or taskkill /T without one.
func killTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	procJobsMu.Lock()
	job, ok := procJobs[cmd]
	delete(procJobs, cmd)
	procJobsMu.Unlock()
	if ok {
		_ = windows.TerminateJobObject(job, 1)
		windows.CloseHandle(job)
		return
	}
	k := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	noConsoleWindow(k)
	if k.Run() != nil {
		_ = cmd.Process.Kill()
	}
}

// releaseJob frees the job of a process that ended by itself (its children may live on, as in a shell).
func releaseJob(cmd *exec.Cmd) {
	procJobsMu.Lock()
	job, ok := procJobs[cmd]
	delete(procJobs, cmd)
	procJobsMu.Unlock()
	if ok {
		windows.CloseHandle(job)
	}
}
