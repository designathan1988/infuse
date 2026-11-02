//go:build windows

package sandbox

// Windows containment: the child runs at Low integrity. Mandatory integrity control is enforced by the
// kernel before any ACL: a Low process cannot write, delete or relabel a Medium object, whatever the
// ACLs say, and every object is Medium unless labeled otherwise. So the rest of the machine is
// read-only for the agent's shell, the project tree is labeled Low, and .infuse, the git hooks and the
// protected files keep an explicit Medium label inside it. Infuse itself runs at Medium and is not
// affected by any of this.

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	startfUseStd = 0x100
	infinite     = 0xFFFFFFFF
)

func platformAvailable() bool { return true }

// RunWrapper contains the child process and runs argv inside it, returning its exit code. It fails
// closed: containment is a guarantee, so a command that cannot be contained does not run. The user
// decides otherwise with the containProcesses setting (a filesystem without labels, for example).
func RunWrapper(root string, protected []string, argv []string) int {
	tok, err := lowToken()
	if err != nil {
		return refuse(err)
	}
	defer tok.Close()
	if err := labelTree(root, protected); err != nil {
		return refuse(err)
	}
	return createContained(tok, argv, root)
}

// refuse reports why a command did not run; 126 is the shell's "found but could not be executed".
func refuse(err error) int {
	fmt.Fprintf(os.Stderr, "[infuse] the command was not run: OS containment is unavailable here (%v). The user can turn containProcesses off in the settings.\n", err)
	return 126
}

// createContained starts the child suspended with the Low token, ties it to a job that dies with this
// wrapper (a killed Bash tool leaves nothing behind), and waits for it. The child inherits this
// process's standard handles, so the Bash tool's pipes keep working unchanged.
func createContained(tok windows.Token, argv []string, root string) int {
	argv, extra, err := containedShell(argv)
	if err != nil {
		return refuse(err)
	}
	cmdline, err := windows.UTF16PtrFromString(quoteCommandLine(argv))
	if err != nil {
		return refuse(err)
	}
	env, err := envBlock(childEnv(os.Environ(), append(envCacheVars(root), extra...)))
	if err != nil {
		return refuse(err)
	}
	dir, err := windows.UTF16PtrFromString(cwd())
	if err != nil {
		return refuse(err)
	}
	job, err := killOnCloseJob()
	if err != nil {
		return refuse(err)
	}
	defer windows.CloseHandle(job)
	si := &windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfo{})), Flags: startfUseStd,
		StdInput:  windows.Handle(os.Stdin.Fd()),
		StdOutput: windows.Handle(os.Stdout.Fd()),
		StdErr:    windows.Handle(os.Stderr.Fd())}
	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_NO_WINDOW | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_SUSPENDED)
	if err := windows.CreateProcessAsUser(tok, nil, cmdline, nil, nil, true, flags, env, dir, si, &pi); err != nil {
		return refuse(err)
	}
	defer windows.CloseHandle(pi.Process)
	defer windows.CloseHandle(pi.Thread)
	if err := windows.AssignProcessToJobObject(job, pi.Process); err != nil {
		_ = windows.TerminateProcess(pi.Process, 126)
		return refuse(err)
	}
	if _, err := windows.ResumeThread(pi.Thread); err != nil {
		_ = windows.TerminateProcess(pi.Process, 126)
		return refuse(err)
	}
	return waitCode(pi.Process)
}

// killOnCloseJob is a job whose processes end when its last handle closes: this wrapper's exit.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

func waitCode(h windows.Handle) int {
	s, err := windows.WaitForSingleObject(h, infinite)
	if err != nil || s != windows.WAIT_OBJECT_0 {
		return 1
	}
	var code uint32
	_ = windows.GetExitCodeProcess(h, &code)
	return int(code)
}

// childEnv layers the cache redirection over the inherited environment. A command that points the
// shell at another root (cd elsewhere) still inherits these; they are per-infuse-process values.
func childEnv(env []string, overrides [][2]string) []string {
	out := env
	for _, kv := range overrides {
		out = replaceEnv(out, kv[0], kv[1])
	}
	return out
}

func replaceEnv(env []string, key, val string) []string {
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), strings.ToUpper(prefix)) {
			env[i] = prefix + val
			return env
		}
	}
	return append(env, prefix+val)
}

func cwd() string {
	d, err := os.Getwd()
	if err != nil {
		d = "."
	}
	return d
}

// quoteCommandLine quotes argv the way the command line MSVCRT rules require.
func quoteCommandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		parts[i] = windows.EscapeArg(a)
	}
	return strings.Join(parts, " ")
}

// envBlock builds the double-null-terminated UTF-16 environment block CreateProcess expects. The
// block's NULs are the format, so the encoding goes through UTF16Encode, not UTF16PtrFromString
// (which refuses interior NULs).
func envBlock(env []string) (*uint16, error) {
	var flat []uint16
	for _, e := range env {
		enc, err := windows.UTF16FromString(e)
		if err != nil {
			continue
		}
		flat = append(flat, enc[:len(enc)-1]...) // UTF16FromString's terminator is not part of the entry
		flat = append(flat, 0)
	}
	flat = append(flat, 0)
	return &flat[0], nil
}
