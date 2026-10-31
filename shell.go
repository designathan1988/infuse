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
	guarded, guardNotes, gerr := newGuard(root, EffectiveSettings(root).ProtectedPaths).GuardCommand(original, cwd)
	if gerr != nil {
		fmt.Fprintln(os.Stderr, "[infuse] "+gerr.Error())
		return 1
	}
	original = guarded
	var tr TransformResult
	if rules.Disabled["heredoc"] {
		tr = TransformResult{Command: original}
	} else {
		tr = TransformCommand(original, cwd)
	}
	command, ruleLines := ApplyCommandRules(root, rules, tr.Command)

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
	trailer := buildTrailer(root, cwd, original, tr.Actions, started, rules, append(guardNotes, ruleLines...))
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

func buildTrailer(root, cwd, original string, actions []TransformAction, started int64, rules *RuleSet, ruleLines []string) string {
	lines := append([]string{}, ruleLines...)
	for _, a := range actions {
		switch a.Kind {
		case ActHeredocToFile:
			lines = append(lines, "[infuse] "+a.Detail)
		case ActLesson:
			lines = append(lines, fmt.Sprintf("[infuse] vice %s: %s", a.Vice, a.Detail))
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
		lines = append(lines, "[infuse] written during this command: "+strings.Join(shown, ", ")+more)
		if m != nil {
			for i, w := range order {
				if i >= 8 {
					break
				}
				if d := DescribeDependents(m, w); d != "" {
					lines = append(lines, d)
				}
			}
		}
		abs := make([]string, 0, len(order))
		for _, w := range order {
			abs = append(abs, filepath.Join(root, filepath.FromSlash(w)))
		}
		lines = append(lines, ApplyWriteRules(root, rules, abs)...)
	}

	if listed, ok := listedDirectory(original); ok && m != nil {
		target := resolveIn(cwd, FromMsysPath(listed))
		if st, err := os.Stat(target); err == nil && st.IsDir() {
			rel := relTo(root, target)
			if rel == "" {
				rel = "."
			}
			if d := DescribeDir(m, rel); d != "" {
				lines = append(lines, d)
			}
			lines = append(lines, ApplyListRules(root, rules, rel)...)
		}
	}
	return strings.Join(lines, "\n")
}

var (
	reSplitSeg = regexp.MustCompile(`&&|\|\||;|\||\r?\n`)
	reCd       = regexp.MustCompile(`^cd\s+(\S+)`)
	reLs       = regexp.MustCompile(`^(?:ls|tree|dir)\b(.*)$`)
)

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
