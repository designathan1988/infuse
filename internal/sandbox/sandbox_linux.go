//go:build linux

package sandbox

// Linux containment, in two stages. Landlock confines writes to the project, the temp folders and the
// devices a shell needs; it cannot subtract a path from a grant, so the protected paths inside the
// project are covered first by a private mount namespace that binds them read-only. Where unprivileged
// user namespaces are disabled, the fallback grants the project's top-level entries one by one, minus
// the protected ones: the kernel still refuses writes to infuse's records, and only creating a new
// top-level file is lost.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const landlockCreateRulesetVersion = 0x1

// landlockFileRights are the rights Landlock accepts on a rule for a file; the rest are directory rights.
const landlockFileRights = unix.LANDLOCK_ACCESS_FS_WRITE_FILE

// landlockWriteRights are the operations a write-restricted project may do: everything that changes the tree.
const landlockWriteRights = unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR |
	unix.LANDLOCK_ACCESS_FS_MAKE_REG | unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO |
	unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK | unix.LANDLOCK_ACCESS_FS_MAKE_SYM

func platformAvailable() bool {
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, landlockCreateRulesetVersion)
	return errno == 0 && abi >= 1
}

// RunWrapper contains argv and runs it, failing closed. The first stage re-executes this binary inside
// a new user and mount namespace; the second, inside it, makes the protected paths read-only.
func RunWrapper(root string, protected []string, argv []string) int {
	if os.Getenv("INFUSE_SANDBOX_STAGE") != "inner" {
		if code, ok := runInNamespace(root, protected, argv); ok {
			return code
		}
		return enter(root, protected, argv, false)
	}
	return enter(root, protected, argv, true)
}

// runInNamespace re-executes the role in private namespaces. ok=false means the kernel refused them
// before the command started, and the fallback applies.
func runInNamespace(root string, protected []string, argv []string) (int, bool) {
	exe, err := os.Executable()
	if err != nil {
		return 0, false
	}
	args := []string{Role, "-r", root}
	for _, p := range protected {
		args = append(args, "-d", p)
	}
	cmd := exec.Command(exe, append(append(args, "--"), argv...)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "INFUSE_SANDBOX_STAGE=inner")
	uid, gid := os.Getuid(), os.Getgid()
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
	}
	if err := cmd.Start(); err != nil {
		return 0, false
	}
	err = cmd.Wait()
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode(), true
	}
	if err != nil {
		return 127, true
	}
	return 0, true
}

// enter applies the containment to this process and replaces it with argv.
func enter(root string, protected []string, argv []string, namespaced bool) int {
	_ = os.Unsetenv("INFUSE_SANDBOX_STAGE")
	if namespaced {
		if err := bindReadOnly(root, protectedPaths(root, protected)); err != nil {
			namespaced = false
			fmt.Fprintf(os.Stderr, "[infuse] read-only mounts unavailable, using per-entry grants: %v\n", err)
		}
	}
	for _, kv := range envCacheVars(root) { // the temp and the caches a shell needs live in the grant
		_ = os.Setenv(kv[0], kv[1])
	}
	if err := restrictSelf(root, protected, namespaced); err != nil {
		fmt.Fprintf(os.Stderr, "[infuse] the command was not run: OS containment is unavailable here (%v). The user can turn containProcesses off in the settings.\n", err)
		return 126
	}
	path, err := execPath(argv[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "[infuse] sandbox: %v\n", err)
		return 127
	}
	if err := unix.Exec(path, argv, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "[infuse] sandbox exec: %v\n", err)
		return 127
	}
	return 0
}

// protectedPaths are the absolute paths kept out of reach: infuse's folder, the git hooks and config,
// and the protected list.
func protectedPaths(root string, protected []string) []string {
	out := []string{filepath.Join(root, ".infuse"), filepath.Join(root, ".git", "hooks"), filepath.Join(root, ".git", "config")}
	for _, rel := range protectedRels(root, protected) {
		if rel = strings.Trim(rel, "/"); !strings.HasPrefix(rel, ".infuse/") {
			out = append(out, filepath.Join(root, filepath.FromSlash(rel)))
		}
	}
	return out
}

// bindReadOnly binds each existing path onto itself and remounts it read-only, in this namespace only.
func bindReadOnly(root string, paths []string) error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return err
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := unix.Mount(p, p, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		flags := uintptr(unix.MS_REMOUNT | unix.MS_BIND | unix.MS_RDONLY | unix.MS_NOSUID | unix.MS_NODEV)
		if err := unix.Mount("", p, "", flags, ""); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

// restrictSelf builds the ruleset and locks the current process in. With the read-only mounts in
// place the whole project is granted; without them, each top-level entry that is not protected.
func restrictSelf(root string, protected []string, namespaced bool) error {
	handled := uint64(landlockWriteRights)
	abi, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		return fmt.Errorf("landlock probe: %v", errno)
	}
	if abi >= 2 {
		handled |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		handled |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	attr := unix.LandlockRulesetAttr{Handled_access_fs: handled}
	fd, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return fmt.Errorf("landlock ruleset: %v", errno)
	}
	defer unix.Close(int(fd))
	if err := grantProject(int(fd), root, protected, handled, namespaced); err != nil {
		return err
	}
	for _, p := range scratchPaths(root) {
		if err := grant(int(fd), p, handled); err != nil {
			return err
		}
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return err
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, fd, 0, 0); errno != 0 {
		return fmt.Errorf("landlock restrict: %v", errno)
	}
	return nil
}

func grantProject(fd int, root string, protected []string, handled uint64, namespaced bool) error {
	if namespaced {
		return grant(fd, root, handled)
	}
	denied := map[string]bool{".infuse": true}
	for _, p := range protectedRels(root, protected) {
		denied[strings.SplitN(strings.Trim(p, "/"), "/", 2)[0]] = true
	}
	// creating and removing direct children stays possible: the rights a directory rule may carry for
	// its own entries, without the write right that would reach into a protected child
	top := handled &^ uint64(unix.LANDLOCK_ACCESS_FS_WRITE_FILE|unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	if err := grant(fd, root, top); err != nil {
		return err
	}
	for _, name := range rootEntries(root) {
		if !denied[name] {
			if err := grant(fd, filepath.Join(root, name), handled); err != nil {
				return err
			}
		}
	}
	return nil
}

// scratchPaths are the writable places a shell needs besides the project: the devices and the private
// temp, where the toolchain caches also live. The shared temp and the home folder stay out of reach.
func scratchPaths(root string) []string {
	return []string{sandboxTemp(root), "/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom", "/dev/tty", "/dev/shm", "/dev/pts"}
}

// grant allows the handled operations beneath path. A rule on a regular file or a device may only
// carry file rights: directory rights there make the kernel refuse the rule with EINVAL. A path that
// does not exist needs no rule.
func grant(rulesetFD int, path string, handled uint64) error {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	access := handled
	if !st.IsDir() {
		access = handled & (landlockFileRights | unix.LANDLOCK_ACCESS_FS_TRUNCATE)
	}
	if access == 0 {
		return nil
	}
	rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
	if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(rulesetFD),
		unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&rule)), 0, 0, 0); errno != 0 {
		return fmt.Errorf("landlock rule %s: %v", path, errno)
	}
	return nil
}

// rootEntries lists the top-level entries of root, sorted for determinism.
func rootEntries(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

func execPath(name string) (string, error) {
	if strings.ContainsRune(name, '/') {
		return name, nil
	}
	return exec.LookPath(name)
}
