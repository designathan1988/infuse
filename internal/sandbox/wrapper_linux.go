//go:build linux && (amd64 || arm64)

package sandbox

// Landlock ruleset for an agent command: reads everywhere, writes only inside the work root and the
// sandbox temporary directory. .infuse is never granted, so no command can rewrite the records.
// Reference: Linux landlock(7), ABI 1 (5.13); refer (ABI 2) and truncate (ABI 3) are used when present.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/lang"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const (
	llCreateRuleset        = 444
	llAddRule              = 445
	llRestrictSelf         = 446
	llRulePathBeneath      = 1
	llCreateRulesetVersion = 1
	oPath                  = 0x200000
	prSetNoNewPrivs        = 38

	fsExecute    = 1 << 0
	fsWriteFile  = 1 << 1
	fsReadFile   = 1 << 2
	fsReadDir    = 1 << 3
	fsRemoveDir  = 1 << 4
	fsRemoveFile = 1 << 5
	fsMakeChar   = 1 << 6
	fsMakeDir    = 1 << 7
	fsMakeReg    = 1 << 8
	fsMakeSock   = 1 << 9
	fsMakeFifo   = 1 << 10
	fsMakeBlock  = 1 << 11
	fsMakeSym    = 1 << 12
	fsRefer      = 1 << 13
	fsTruncate   = 1 << 14
)

type rulesetAttr struct{ handledAccessFS uint64 }

type pathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
	_             uint32
}

func supportedArch() bool { return runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" }

func masks(abi int) (handled, readSet, dirSet, fileSet uint64) {
	handled = (1 << 13) - 1 // v1: bits 0..12
	if abi >= 2 {
		handled |= fsRefer
	}
	if abi >= 3 {
		handled |= fsTruncate
	}
	readSet = fsExecute | fsReadFile | fsReadDir
	dirSet = handled
	fileSet = readSet | fsWriteFile
	if abi >= 3 {
		fileSet |= fsTruncate
	}
	return
}

// RunWrapper applies the sandbox to this process and becomes the requested command.
func RunWrapper(args []string) int {
	if len(args) < 4 || args[2] != "--" {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox: malformed wrapper arguments")
		return 126
	}
	root := args[0]
	var protected []string
	if json.Unmarshal([]byte(args[1]), &protected) != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox: malformed protected-path list")
		return 126
	}
	command := args[3:]
	if abs, err := exec.LookPath(command[0]); err == nil {
		command[0] = abs
	}
	if err := apply(root, protected); err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox:", err)
		return 126
	}
	tmp := filepath.Join(core.InfuseHome(), "sandbox", "tmp")
	if err := os.MkdirAll(tmp, 0o700); err == nil {
		for _, k := range []string{"TMPDIR", "TMP", "TEMP"} {
			_ = os.Setenv(k, tmp)
		}
	}
	if err := syscall.Exec(command[0], command, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox: exec:", err)
		return 126
	}
	return 0
}

func apply(root string, protected []string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return fmt.Errorf("work root %q is not a directory", root)
	}
	abi, err := landlockABI()
	if err != nil || abi < 1 {
		return fmt.Errorf("landlock is unavailable on this kernel; refusing to run the command outside the sandbox (%v)", err)
	}
	handled, readSet, dirSet, fileSet := masks(abi)
	if err := unix.Prctl(prSetNoNewPrivs, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
	}
	ruleset, _, errno := unix.Syscall(llCreateRuleset, uintptr(unsafe.Pointer(&rulesetAttr{handled})), unsafe.Sizeof(rulesetAttr{}), 0)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset: %w", errno)
	}
	defer unix.Close(int(ruleset))
	if err := addPathRule(int(ruleset), "/", readSet); err != nil {
		return err
	}
	// The root itself may create/rename/delete its direct entries, but may not write files beneath it:
	// that is what keeps .infuse out of reach while still allowing new top-level files and folders.
	var rootSet uint64 = fsReadDir | fsMakeReg | fsMakeDir | fsMakeSym | fsMakeSock | fsRemoveFile | fsRefer
	if abi < 2 {
		rootSet &^= fsRefer
	}
	if err := addPathRule(int(ruleset), root, rootSet); err != nil {
		return err
	}
	if err := grantTree(int(ruleset), root, protected, dirSet, fileSet); err != nil {
		return err
	}
	tmp := filepath.Join(core.InfuseHome(), "sandbox", "tmp")
	if err := os.MkdirAll(tmp, 0o700); err == nil {
		_ = addPathRule(int(ruleset), tmp, dirSet)
	}
	for _, dev := range []string{"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty"} {
		if _, err := os.Stat(dev); err == nil {
			_ = addPathRule(int(ruleset), dev, fsWriteFile|fsReadFile|fsTruncate)
		}
	}
	runtime.LockOSThread() // the domain belongs to the thread; exec happens on it
	if _, _, errno := unix.Syscall(llRestrictSelf, ruleset, 0, 0); errno != 0 {
		return fmt.Errorf("landlock_restrict_self: %w", errno)
	}
	return nil
}

func landlockABI() (int, error) {
	r, _, errno := unix.Syscall(llCreateRuleset, 0, 0, llCreateRulesetVersion)
	if errno != 0 {
		return 0, errno
	}
	return int(r), nil
}

func addPathRule(ruleset int, path string, allowed uint64) error {
	if allowed == 0 {
		return nil
	}
	fd, err := unix.Open(path, oPath|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil // unreadable now: granting nothing is the safe direction
	}
	defer unix.Close(fd)
	attr := pathBeneathAttr{allowedAccess: allowed, parentFD: int32(fd)}
	if _, _, errno := unix.Syscall6(llAddRule, uintptr(ruleset), llRulePathBeneath, uintptr(unsafe.Pointer(&attr)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("landlock_add_rule(%s): %w", path, errno)
	}
	return nil
}

// grantTree grants write inside every directory of root except .infuse and the user's protected paths.
func grantTree(ruleset int, root string, protected []string, dirSet, fileSet uint64) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isProtected(rel, protected) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			_ = addPathRule(ruleset, p, dirSet)
			return nil
		}
		if !strings.Contains(rel, "/") { // files deeper are covered by their directory's rule
			_ = addPathRule(ruleset, p, fileSet)
		}
		return nil
	})
}

func isProtected(rel string, protected []string) bool {
	if rel == ".infuse" || strings.HasPrefix(rel, ".infuse/") {
		return true
	}
	for _, pat := range protected {
		base := strings.TrimSuffix(filepath.ToSlash(pat), "/**")
		if rel == base || strings.HasPrefix(rel, base+"/") || lang.GlobToRegex(pat).MatchString(rel) {
			return true
		}
	}
	return false
}
