package filter

// The interposed shell. Every agent's Bash tool invokes `bash -c "<command>"` (sometimes -lc, -ec...).
// infuse installs itself under the name bash(.exe); when invoked that way, or as `infuse shell`:
//  1. the command is transformed (no heredoc reaches bash; user rules apply);
//  2. the real bash runs it with the same options;
//  3. the output goes back infused: what was written, who depends on it, the map of what was listed.
// Without `-c` (interactive/login use) it simply runs the real bash.

import (
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/platform/process"
	"infuse/internal/projectmap"
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
	realBash := core.FindRealBash()
	cwd, _ := os.Getwd()
	root := core.FindRoot(cwd)

	cIndex := shellCommandIndex(args)
	if cIndex == -1 || cIndex+1 >= len(args) {
		return passthrough(realBash, args)
	}

	started := time.Now().UnixMilli()
	original := args[cIndex+1]
	startShellWatcher(root)

	rules := LoadRules(root)
	guarded, guardNotes, refused := guardShellCommand(root, cwd, original, policyFor)
	if refused != 0 {
		return refused
	}
	original = guarded
	tr := applyCommandTransform(original, cwd, rules)
	command, ruleLines := ApplyCommandRulesMsgs(root, rules, tr.Command)
	recordCommand(root, original, tr)

	newArgs := append([]string{}, args...)
	newArgs[cIndex+1] = command
	code := passthrough(realBash, newArgs)
	finishShellRun(root, cwd, original, tr.Actions, started, rules, append(guardNotes, ruleLines...))
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

// guardShellCommand inspects the command; a non-zero result is the exit code after reporting a refusal.
func guardShellCommand(root, cwd, original string, policyFor func(root string, protected []string) Policy) (string, []core.Msg, int) {
	guarded, notes, gerr := NewGuard(root, policyFor(root, config.EffectiveSettings(root).ProtectedPaths)).GuardCommandMsgs(original, cwd)
	if gerr == nil {
		return guarded, notes, 0
	}
	var why any = gerr.Error()
	if m, _ := core.MsgOf(gerr); !m.Empty() {
		why = m
	}
	fmt.Fprintln(os.Stderr, core.M("shell.refused", "why", why).String())
	return "", nil, 1
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
			journal.Lesson(root, a.Vice, a.Detail)
		} else {
			journal.Journal(root, journal.JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}
}

// finishShellRun refreshes the map when no watcher did it and appends the trailer to the output.
func finishShellRun(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, notes []core.Msg) {
	if _, alive := WatcherAlive(root); !alive {
		projectmap.WriteMap(root, projectmap.BuildMap(root)) // no watcher here: keep the map honest ourselves
	}
	trailer := joinMsgLines(TrailerMsgs(root, cwd, original, actions, started, rules, notes))
	if trailer != "" {
		_, _ = io.WriteString(os.Stdout, "\n"+trailer+"\n")
	}
}

func passthrough(realBash string, args []string) int {
	cmd := exec.Command(realBash, args...)
	process.NoConsoleWindow(cmd)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	return exitCode(cmd.Run())
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

// buildTrailer is what infuse appends to a command's output, in English (trailerMsgs gives the
// messages); ruleLines are lines already written.
func BuildTrailer(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	var notes []core.Msg
	for _, l := range ruleLines {
		notes = append(notes, core.M("text.verbatim", "text", l))
	}
	return joinMsgLines(TrailerMsgs(root, cwd, original, actions, started, rules, notes))
}

// joinMsgLines is the English of messages, one per line.
func joinMsgLines(ms []core.Msg) string {
	return strings.Join(MsgStrings(ms), "\n")
}

// trailerMsgs is what infuse appends to a command's output: notes (the guard's, the rules'), what was
// written and who depends on it, the map of a listed folder. A caller with a session records each one
// with the call, so the user reads them in their language.
func TrailerMsgs(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, notes []core.Msg) []core.Msg {
	lines := append([]core.Msg{}, notes...)
	lines = append(lines, actionMessages(actions)...)

	m := projectmap.LoadOrBuildMap(root)
	if order := writtenPaths(root, actions, started); len(order) > 0 {
		lines = append(lines, writtenMessages(root, m, order, rules)...)
	}
	return append(lines, listedDirMessages(root, cwd, original, rules, m)...)
}

// actionMessages is the trailer's note per transformation.
func actionMessages(actions []TransformAction) []core.Msg {
	var lines []core.Msg
	for _, a := range actions {
		switch a.Kind {
		case ActHeredocToFile:
			lines = append(lines, core.M("shell.action", "detail", a.Detail))
		case ActLesson:
			lines = append(lines, core.M("shell.vice", "vice", a.Vice, "detail", a.Detail))
		}
	}
	return lines
}

// writtenPaths lists, in order and without repeats, the files written during the command.
func writtenPaths(root string, actions []TransformAction, started int64) []string {
	written := map[string]bool{}
	var order []string
	add := func(rel string) {
		if !written[rel] {
			written[rel] = true
			order = append(order, rel)
		}
	}
	for _, e := range journal.ReadJournalSince(root, started) {
		if e.Kind == "write" && e.Path != "" {
			add(e.Path)
		}
	}
	for _, a := range actions {
		if a.Kind == ActHeredocToFile && a.Path != "" {
			add(core.RelTo(root, a.Path))
		}
	}
	return order
}

// writtenMessages is what the trailer says about the files written: the list, who depends on them and
// the write rules that matched.
func writtenMessages(root string, m *projectmap.ProjectMap, order []string, rules *RuleSet) []core.Msg {
	var lines []core.Msg
	shown := order
	more := ""
	if len(shown) > 20 {
		shown, more = shown[:20], "…"
	}
	lines = append(lines, core.M("shell.written", "files", strings.Join(shown, ", ")+more))
	if m != nil {
		for i, w := range order {
			if i >= 8 {
				break
			}
			if d := projectmap.DependentsMsg(m, w); !d.Empty() {
				lines = append(lines, d)
			}
		}
	}
	abs := make([]string, 0, len(order))
	for _, w := range order {
		abs = append(abs, filepath.Join(root, filepath.FromSlash(w)))
	}
	return append(lines, ApplyWriteRulesMsgs(root, rules, abs)...)
}

// listedDirMessages describes the directory an ls/tree command listed, when it is one.
func listedDirMessages(root, cwd, original string, rules *RuleSet, m *projectmap.ProjectMap) []core.Msg {
	listed, ok := ListedDirectory(original)
	if !ok || m == nil {
		return nil
	}
	if MayChangeTree(original) { // rm, mv, a generator before the ls: describe the folder as it is now
		m = projectmap.BuildMap(root)
		projectmap.WriteMap(root, m)
	}
	target := ResolveIn(cwd, core.FromMsysPath(listed))
	st, err := os.Stat(target)
	if err != nil || !st.IsDir() {
		return nil
	}
	rel := core.RelTo(root, target)
	if rel == "" {
		rel = "."
	}
	var lines []core.Msg
	if d := projectmap.DescribeDir(m, rel); d != "" {
		lines = append(lines, core.M("text.verbatim", "text", d)) // the map's listing is data, the same in every language
	}
	return append(lines, ApplyListRulesMsgs(root, rules, rel)...)
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
