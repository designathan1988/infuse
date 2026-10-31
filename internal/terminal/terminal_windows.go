//go:build windows

package terminal

// ConPTY: a pseudo console hosting the real bash (Git Bash) or, without it, PowerShell.

import (
	"infuse/internal/core"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type conPTY struct {
	hpc     windows.Handle
	in      windows.Handle // we write keystrokes here
	out     windows.Handle // we read output here
	process windows.Handle
	thread  windows.Handle
}

func startPTY(cwd string, cols, rows int) (PTY, error) {
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, err
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		windows.CloseHandle(inR)
		windows.CloseHandle(inW)
		return nil, err
	}
	var hpc windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: int16(cols), Y: int16(rows)}, inR, outW, 0, &hpc); err != nil {
		for _, h := range []windows.Handle{inR, inW, outR, outW} {
			windows.CloseHandle(h)
		}
		return nil, err
	}
	// the pseudo console owns its ends now
	windows.CloseHandle(inR)
	windows.CloseHandle(outW)

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, err
	}
	defer attrs.Delete()
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, *(*unsafe.Pointer)(unsafe.Pointer(&hpc)), unsafe.Sizeof(hpc)); err != nil {
		return nil, err
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	si.Flags = windows.STARTF_USESTDHANDLES // no inherited std handles: the pseudo console provides them

	cmdline := shellCommandLine()
	cmd, _ := windows.UTF16PtrFromString(cmdline)
	dir, _ := windows.UTF16PtrFromString(cwd)
	env := envBlock(append(os.Environ(), "TERM=xterm-256color", "CHERE_INVOKING=1", "COLORTERM=truecolor"))
	var pi windows.ProcessInformation
	err = windows.CreateProcess(nil, cmd, nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, &env[0], dir, &si.StartupInfo, &pi)
	if err != nil {
		windows.ClosePseudoConsole(hpc)
		windows.CloseHandle(inW)
		windows.CloseHandle(outR)
		return nil, err
	}
	return &conPTY{hpc: hpc, in: inW, out: outR, process: pi.Process, thread: pi.Thread}, nil
}

func shellCommandLine() string {
	bash := core.FindRealBash()
	if strings.HasSuffix(strings.ToLower(bash), ".exe") && core.Exists(bash) {
		return `"` + bash + `" --login -i`
	}
	return "powershell.exe -NoLogo"
}

func envBlock(env []string) []uint16 {
	var b []uint16
	for _, kv := range env {
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	return append(b, 0)
}

func (c *conPTY) Read(p []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(c.out, p, &n, nil)
	if err == windows.ERROR_BROKEN_PIPE {
		return int(n), syscall.EPIPE
	}
	return int(n), err
}

func (c *conPTY) Write(p []byte) (int, error) {
	var n uint32
	err := windows.WriteFile(c.in, p, &n, nil)
	return int(n), err
}

func (c *conPTY) Resize(cols, rows int) error {
	return windows.ResizePseudoConsole(c.hpc, windows.Coord{X: int16(cols), Y: int16(rows)})
}

func (c *conPTY) Close() error {
	_ = windows.TerminateProcess(c.process, 0)
	_ = windows.CancelIoEx(c.out, nil) // a pending ReadFile must not hold the handle while it is closed
	windows.ClosePseudoConsole(c.hpc)
	windows.CloseHandle(c.in)
	windows.CloseHandle(c.out)
	windows.CloseHandle(c.process)
	windows.CloseHandle(c.thread)
	return nil
}
