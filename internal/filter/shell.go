package filter

// The interposed shell. Every agent's Bash tool invokes `bash -c "<command>"` (sometimes -lc, -ec...).
// infuse installs itself under the name bash(.exe); when invoked that way, or as `infuse shell`:
//  1. the command is transformed (no heredoc reaches bash; user rules apply);
//  2. the real bash runs it with the same options, inside the OS sandbox of the project;
//  3. the output goes back infused: what was written, who depends on it, the map of what was listed.
// Without `-c` (bash x.sh, bash -s) nothing can be read before it runs: the guard does not apply, and
// the real bash still runs in the sandbox. Only a person's interactive shell runs as it is.

import (
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
	"infuse/internal/sandbox"
	"infuse/internal/shellparse"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func RunShell(args []string, policyFor func(root string, protected []string) Policy) int {
	if root, ok := watcherRole(args); ok { // the shim started as the project's watcher
		RunWatcher(root, WatcherIdle())
		return 0
	}
	realBash := core.FindRealBash()
	cwd, _ := os.Getwd()
	root := core.FindRoot(cwd)

	cIndex := shellCommandIndex(args)
	if cIndex == -1 || cIndex+1 >= len(args) {
		return passthrough(root, realBash, args)
	}

	started := time.Now().UnixMilli()
	original := args[cIndex+1]
	startShellWatcher(root)

	rules := LoadRules(root)
	guard := NewGuard(root, policyFor(root, config.EffectiveSettings(root).ProtectedPaths))
	guarded, guardNotes, gerr := guard.GuardCommandMsgs(original, cwd)
	if gerr != nil {
		return reportRefusal(gerr)
	}
	original = guarded
	tr := applyCommandTransform(original, cwd, rules)
	rewritten, ruleLines := ApplyCommandRulesMsgs(root, rules, tr.Command)
	command, reguardNotes, gerr := guard.Reguard(tr.Command, rewritten, cwd)
	if gerr != nil {
		return reportRefusal(gerr)
	}
	recordCommand(root, original, tr)

	newArgs := append([]string{}, args...)
	newArgs[cIndex+1] = command
	code := passthrough(root, realBash, newArgs)
	finishShellRun(root, cwd, original, tr.Actions, started, rules, append(append(guardNotes, ruleLines...), reguardNotes...))
	return code
}

// shellCommandIndex returns the index of the shell's -c (or combined -lc…) option, or -1.
func shellCommandIndex(args []string) int {
	for i, a := range args {
		if a == "-c" || (regexp.MustCompile(`^-[a-zA-Z]+$`).MatchString(a) && strings.Contains(a, "c")) {
			return i
		}
	}
	return -1
}

// startShellWatcher starts the project watcher; the shell must never fail because of it.
func startShellWatcher(root string) {
	defer func() { _ = recover() }()
	EnsureWatcher(root)
	config.RememberProject(root)
}

// reportRefusal tells the agent why the command did not run and returns the shell's exit code.
func reportRefusal(gerr error) int {
	var why any = gerr.Error()
	if m, _ := core.MsgOf(gerr); !m.Empty() {
		why = m
	}
	fmt.Fprintln(os.Stderr, core.M("shell.refused", "why", why).String())
	return 1
}

// applyCommandTransform rewrites the command unless the project disabled the heredoc transform.
func applyCommandTransform(original, cwd string, rules *RuleSet) TransformResult {
	if rules.Disabled["heredoc"] {
		return TransformResult{Command: original}
	}
	return TransformCommand(original, cwd)
}

// recordCommand journals the command and each transformation it produced.
func recordCommand(root, original string, tr TransformResult) {
	detail := ""
	if len(tr.Actions) > 0 {
		detail = fmt.Sprintf("%d transformation(s)", len(tr.Actions))
	}
	journal.Journal(root, journal.JournalEntry{Kind: "command", Command: original, Detail: detail})
	for _, a := range tr.Actions {
		if a.Kind == ActLesson && a.Vice != "" {
			journal.Observe(root, journal.JournalEntry{Kind: "vice", Path: core.RelTo(root, a.Path), Command: original, Detail: a.Detail}, a.Vice)
		} else {
			journal.Journal(root, journal.JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
}

// finishShellRun keeps the map honest and appends the trailer to the output.
func finishShellRun(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, notes []core.Msg) {
	refreshWithoutWatcher(root, actions, started)
	trailer := joinMsgLines(TrailerMsgs(root, cwd, original, actions, started, rules, notes))
	if trailer != "" {
		_, _ = io.WriteString(os.Stdout, "\n"+trailer+"\n")
	}
}

// refreshWithoutWatcher covers a dead watcher: the files known to have changed are applied to the map
// and the watcher comes back, in the background, with a full build of its own. The project is never
// rebuilt in a command's path.
func refreshWithoutWatcher(root string, actions []TransformAction, started int64) {
	if _, alive := WatcherAlive(root); alive {
		return
	}
	projectmap.UpdateFiles(root, writtenPaths(root, actions, started))
	startShellWatcher(root)
}

// passthrough runs the real bash, contained, with what the agent asked for. This binary is the shim, so
// a real bash that turns out to be itself would call itself forever.
func passthrough(root, realBash string, args []string) int {
	if isSelf(realBash) {
		fmt.Fprintln(os.Stderr, core.M("shell.self", "path", realBash).String())
		return 127
	}
	argv := containedArgv(root, realBash, args)
	cmd := exec.Command(argv[0], argv[1:]...)
	process.NoConsoleWindow(cmd)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	return exitCode(cmd.Run())
}

// containedArgv wraps the real bash in the project's sandbox. The wrapper is this same binary under its
// __sandbox role, and a shell started inside the sandbox is not wrapped again (sandbox.Wrap).
func containedArgv(root, realBash string, args []string) []string {
	argv := append([]string{realBash}, args...)
	if interactiveUse(args) || !containable(root) {
		return argv
	}
	return sandbox.Wrap(argv, root, InfuseRecords(), config.EffectiveSettings(root).ContainProcesses)
}

// interactiveUse is a person at a terminal: no script and no command to run, stdin a console.
func interactiveUse(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") || a == "-s" {
			return false
		}
	}
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// containable refuses the folders that are no project. Outside any project FindRoot answers with the
// working directory, and containment makes its root writable to the contained process: done on a drive
// or on the user's home, that would open everything in it instead of closing anything.
func containable(root string) bool {
	root = filepath.Clean(root)
	return filepath.Dir(root) != root && !sameFile(root, core.HomeDir())
}

func isSelf(p string) bool {
	self, err := os.Executable()
	return err == nil && sameFile(self, p)
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	fmt.Fprintln(os.Stderr, "infuse:", err)
	return 127
}

var (
	reCd = regexp.MustCompile(`^cd\s+(\S+)`)
	reLs = regexp.MustCompile(`^(?:ls|tree|dir)\b(.*)$`)
)

// MayChangeTree reports whether a command line has a step beyond reading.
func MayChangeTree(cmd string) bool { return shellparse.MayChangeTree(cmd) }

// listedDirectory returns the directory an ls/tree command is listing.
func ListedDirectory(cmd string) (string, bool) {
	cwdPrefix := ""
	for _, seg := range shellparse.Segments(cmd) {
		seg = strings.TrimSpace(seg)
		if m := reCd.FindStringSubmatch(seg); m != nil {
			cwdPrefix = strings.Trim(m[1], `'"`)
			continue
		}
		if m := reLs.FindStringSubmatch(seg); m != nil {
			target := "."
			for _, a := range strings.Fields(m[1]) {
				if !strings.HasPrefix(a, "-") {
					target = strings.Trim(a, `'"`)
					break
				}
			}
			if cwdPrefix != "" {
				return filepath.Join(cwdPrefix, target), true
			}
			return target, true
		}
	}
	return "", false
}
