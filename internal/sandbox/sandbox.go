// Package sandbox contains the agent's shell processes at the OS level: a command that escapes the
// command parser still cannot write outside the project, and infuse's own records inside it stay
// unreachable, because the kernel refuses the write — not a parser that reads the command first.
//
// Windows runs the child at Low integrity: the project tree carries a Low label, the protected records
// keep a Medium one, and everything else is Medium by default. Linux applies a Landlock ruleset that
// handles only write operations and grants them under the project, and macOS a Seatbelt profile that
// denies every write outside it. Where the platform offers none of these, commands run as before and
// the turn audit still judges what changed.
package sandbox

import (
	"fmt"
	"infuse/internal/platform/fslabel"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// containment args of the __sandbox role: -r <root>, -d <protected rel>, then -- and the real argv.
// Role is the CLI subcommand of the sandbox wrapper (infuse __sandbox).
const Role = "__sandbox"

// containedVar marks the processes that run inside the sandbox.
const containedVar = "INFUSE_CONTAINED"

// Allowed reports whether the user has not turned containment off.
func Allowed(containProcesses *bool) bool {
	return containProcesses == nil || *containProcesses
}

// Available reports whether this platform can contain a child process at the OS level.
func Available() bool { return platformAvailable() }

// Wrap returns the argv to run: when containment is on and available, the command goes through the
// __sandbox role of this binary; otherwise the argv is returned unchanged and the turn audit keeps
// watching what the command changed. INFUSE_NO_SANDBOX=1 is the explicit escape hatch: the tests of
// the other layers set it (their process is the test binary, not the real role), and so can a user
// in trouble.
func Wrap(argv []string, root string, protected []string, containProcesses *bool) []string {
	return WrapIsolated(argv, root, protected, "", containProcesses)
}

// Contained reports whether Wrap would really contain a command right now.
func Contained(containProcesses *bool) bool {
	return Allowed(containProcesses) && Available() && os.Getenv("INFUSE_NO_SANDBOX") != "1"
}

// WrapIsolated is Wrap for code that must not share anything with the agent's shell. With a key the
// temp and toolchain caches live under PrivateDir instead of the project's shared folder, keyed by it,
// and the child does not inherit infuse's own variables.
func WrapIsolated(argv []string, root string, protected []string, key string, containProcesses *bool) []string {
	// A process already inside the sandbox is not wrapped again: the shim can be on its PATH, and a
	// contained process could not write the labels a second wrapper needs.
	if len(argv) == 0 || !Contained(containProcesses) || os.Getenv(containedVar) == "1" {
		return argv
	}
	exe, err := os.Executable()
	if err != nil {
		return argv
	}
	out := []string{exe, Role, "-r", root}
	if key != "" {
		out = append(out, "-k", key)
	}
	for _, p := range protected {
		out = append(out, "-d", p)
	}
	out = append(out, "--")
	return append(out, argv...)
}

// PrivateDir is a folder a contained process can work inside but cannot list: without the names
// beneath it, nothing running at Low integrity outside the executor can find a snapshot or its caches.
func PrivateDir() (string, error) {
	p := filepath.Join(os.TempDir(), "workspaces")
	if err := os.MkdirAll(p, 0o700); err != nil {
		return "", err
	}
	return p, fslabel.NoReadUp(p)
}

// isolationKey is the -k value of this __sandbox process ("" for the agent's shell).
var isolationKey string

// homeOverride keeps the infuse home known after an isolated child's environment is cleaned.
var homeOverride string

// dropInfuseEnv removes infuse's variables from what the child inherits.
func dropInfuseEnv() {
	homeOverride, _ = infuseHome()
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(strings.ToUpper(name), "INFUSE_") {
			_ = os.Unsetenv(name)
		}
	}
}

// Main is the __sandbox role: it contains the current process and runs what follows "--".
func Main(args []string) int {
	root := ""
	var protected []string
	argv := []string{}
	past := false
	for i := 0; i < len(args); i++ {
		switch {
		case past:
			argv = append(argv, args[i])
		case args[i] == "--":
			past = true
		case args[i] == "-r" && i+1 < len(args):
			i++
			root = args[i]
		case args[i] == "-d" && i+1 < len(args):
			i++
			protected = append(protected, args[i])
		case args[i] == "-k" && i+1 < len(args):
			i++
			isolationKey = args[i]
		default:
			fmt.Fprintf(os.Stderr, "[infuse] __sandbox: unexpected argument %q\n", args[i])
			return 2
		}
	}
	if root == "" || len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "[infuse] __sandbox: need -r <root> and a command")
		return 2
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() { // fail closed, never run outside the sandbox
		fmt.Fprintf(os.Stderr, "[infuse] the command was not run: the work root %q is not a folder.\n", root)
		return 126
	}
	if isolationKey != "" {
		dropInfuseEnv() // an isolated child has no infuse in it to mark, and must not learn where it runs
	} else {
		_ = os.Setenv(containedVar, "1")
	}
	return RunWrapper(root, protected, argv)
}

// infuseHome is the user's infuse folder.
func infuseHome() (string, error) {
	if homeOverride != "" {
		return homeOverride, nil
	}
	if home := os.Getenv("INFUSE_HOME"); home != "" {
		return home, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".infuse"), nil
}

// runUncontained runs argv as it is and returns its exit code.
func runUncontained(argv []string) int {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		return 127
	}
	return 0
}

// LabelNoReadUp keeps path unreadable by contained processes: signing keys and anchors, which a
// contained shell could otherwise read and use to forge what they protect.
func LabelNoReadUp(path string) error { return fslabel.NoReadUp(path) }
