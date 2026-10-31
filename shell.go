package main

// The interposed shell. Every agent's Bash tool invokes `bash -c "<command>"` (sometimes -lc, -ec...).
// infuse installs itself under the name bash(.exe); when invoked that way, or as `infuse shell`:
//  1. the command is transformed (no heredoc reaches bash; user rules apply);
//  2. the real bash runs it with the same options;
//  3. the output goes back infused: what was written, who depends on it, the map of what was listed.
// Without `-c` (interactive/login use) it simply runs the real bash.

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func RunShell(args []string) int {
	realBash := FindRealBash()
	cwd, _ := os.Getwd()
	root := FindRoot(cwd)

	cIndex := -1
	for i, a := range args {
		if a == "-c" || (regexp.MustCompile(`^-[a-zA-Z]+$`).MatchString(a) && strings.Contains(a, "c")) {
			cIndex = i
			break
		}
	}
	if cIndex == -1 || cIndex+1 >= len(args) {
		return passthrough(realBash, args)
	}

	started := time.Now().UnixMilli()
	original := args[cIndex+1]
	func() {
		defer func() { _ = recover() }() // the shell must never fail because of the watcher
		EnsureWatcher(root)
		RememberProject(root)
	}()

	rules := LoadRules(root)
	guarded, guardNotes, gerr := newGuard(root, EffectiveSettings(root).ProtectedPaths).GuardCommandMsgs(original, cwd)
	if gerr != nil {
		var why any = gerr.Error()
		if m := msgOf(gerr); !m.Empty() {
			why = m
		}
		fmt.Fprintln(os.Stderr, M("shell.refused", "why", why).String())
		return 1
	}
	original = guarded
	var tr TransformResult
	if rules.Disabled["heredoc"] {
		tr = TransformResult{Command: original}
	} else {
		tr = TransformCommand(original, cwd)
	}
	command, ruleLines := ApplyCommandRulesMsgs(root, rules, tr.Command)

	detail := ""
	if len(tr.Actions) > 0 {
		detail = fmt.Sprintf("%d transformation(s)", len(tr.Actions))
	}
	Journal(root, JournalEntry{Kind: "command", Command: original, Detail: detail})
	for _, a := range tr.Actions {
		if a.Kind == ActLesson && a.Vice != "" {
			Lesson(root, a.Vice, a.Detail)
		} else {
			Journal(root, JournalEntry{Kind: "transform", Path: a.Path, Bytes: a.Bytes, Detail: a.Detail})
		}
	}

	newArgs := append([]string{}, args...)
	newArgs[cIndex+1] = command
	cmd := exec.Command(realBash, newArgs...)
	noConsoleWindow(cmd)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	err := cmd.Run()
	code := exitCode(err)

	if _, alive := watcherAlive(root); !alive {
		WriteMap(root, BuildMap(root)) // no watcher here: keep the map honest ourselves
	}
	trailer := joinMsgLines(trailerMsgs(root, cwd, original, tr.Actions, started, rules, append(guardNotes, ruleLines...)))
	if trailer != "" {
		_, _ = io.WriteString(os.Stdout, "\n"+trailer+"\n")
	}
	return code
}

func passthrough(realBash string, args []string) int {
	cmd := exec.Command(realBash, args...)
	noConsoleWindow(cmd)
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
func buildTrailer(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	var notes []Msg
	for _, l := range ruleLines {
		notes = append(notes, M("text.verbatim", "text", l))
	}
	return joinMsgLines(trailerMsgs(root, cwd, original, actions, started, rules, notes))
}

// joinMsgLines is the English of messages, one per line.
func joinMsgLines(ms []Msg) string {
	return strings.Join(msgStrings(ms), "\n")
}

// trailerMsgs is what infuse appends to a command's output: notes (the guard's, the rules'), what was
// written and who depends on it, the map of a listed folder. A caller with a session records each one
// with the call, so the user reads them in their language.
func trailerMsgs(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, notes []Msg) []Msg {
	lines := append([]Msg{}, notes...)
	for _, a := range actions {
		switch a.Kind {
		case ActHeredocToFile:
			lines = append(lines, M("shell.action", "detail", a.Detail))
		case ActLesson:
			lines = append(lines, M("shell.vice", "vice", a.Vice, "detail", a.Detail))
		}
	}

	m := LoadOrBuildMap(root)
	written := map[string]bool{}
	var order []string
	add := func(rel string) {
		if !written[rel] {
			written[rel] = true
			order = append(order, rel)
		}
	}
	for _, e := range ReadJournalSince(root, started) {
		if e.Kind == "write" && e.Path != "" {
			add(e.Path)
		}
	}
	for _, a := range actions {
		if a.Kind == ActHeredocToFile && a.Path != "" {
			add(relTo(root, a.Path))
		}
	}
	if len(order) > 0 {
		shown := order
		more := ""
		if len(shown) > 20 {
			shown, more = shown[:20], "…"
		}
		lines = append(lines, M("shell.written", "files", strings.Join(shown, ", ")+more))
		if m != nil {
			for i, w := range order {
				if i >= 8 {
					break
				}
				if d := DependentsMsg(m, w); !d.Empty() {
					lines = append(lines, d)
				}
			}
		}
		abs := make([]string, 0, len(order))
		for _, w := range order {
			abs = append(abs, filepath.Join(root, filepath.FromSlash(w)))
		}
		lines = append(lines, ApplyWriteRulesMsgs(root, rules, abs)...)
	}

	if listed, ok := listedDirectory(original); ok && m != nil {
		if mayChangeTree(original) { // rm, mv, a generator before the ls: describe the folder as it is now
			m = BuildMap(root)
			WriteMap(root, m)
		}
		target := resolveIn(cwd, FromMsysPath(listed))
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			rel := relTo(root, target)
			if rel == "" {
				rel = "."
			}
			if d := DescribeDir(m, rel); d != "" {
				lines = append(lines, M("text.verbatim", "text", d)) // the map's listing is data, the same in every language
			}
			lines = append(lines, ApplyListRulesMsgs(root, rules, rel)...)
		}
	}
	return lines
}

var (
	reSplitSeg = regexp.MustCompile(`&&|\|\||;|\||\r?\n`)
	reCd       = regexp.MustCompile(`^cd\s+(\S+)`)
	reLs       = regexp.MustCompile(`^(?:ls|tree|dir)\b(.*)$`)
)

// mayChangeTree reports whether a command line has a step beyond reading (it may add, move or remove files).
func mayChangeTree(cmd string) bool {
	readOnly := map[string]bool{"ls": true, "tree": true, "dir": true, "cd": true, "pwd": true, "echo": true, "cat": true, "head": true, "tail": true, "grep": true, "rg": true, "wc": true, "find": true, "stat": true, "file": true, "du": true, "git": false}
	for _, seg := range reSplitSeg.Split(cmd, -1) {
		f := strings.Fields(seg)
		if len(f) > 0 && !readOnly[f[0]] {
			return true
		}
	}
	return false
}

// listedDirectory returns the directory an ls/tree command is listing.
func listedDirectory(cmd string) (string, bool) {
	cwdPrefix := ""
	for _, seg := range reSplitSeg.Split(cmd, -1) {
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

func relTo(root, abs string) string {
	r, err := filepath.Rel(root, abs)
	if err != nil {
		return toPosix(abs)
	}
	if r == "." {
		return ""
	}
	return toPosix(r)
}
