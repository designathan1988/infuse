// Package shellparse classifies shell command lines by what they do to the working tree. It owns the
// segment split and the command tables used by the filter, the diagnosis gate and the shell trailer, so
// those places cannot drift apart again.
package shellparse

import (
	"regexp"
	"strings"
)

var splitRe = regexp.MustCompile(`&&|\|\||;|\||\r?\n`)

// Segments splits a command line into the simple commands it runs.
func Segments(cmd string) []string {
	return splitRe.Split(cmd, -1)
}

// Fields splits one segment into its words.
func Fields(segment string) []string {
	return strings.Fields(segment)
}

// readCommands only read: they never add, move or remove files.
var readCommands = map[string]bool{
	"cat": true, "cd": true, "dir": true, "du": true, "echo": true, "file": true, "find": true,
	"grep": true, "head": true, "ls": true, "pwd": true, "rg": true, "stat": true, "tail": true,
	"tree": true, "type": true, "wc": true, "where": true, "which": true,
}

// diagnosisGit are the git subcommands the diagnosis gate lets through unchanged (version-control
// bookkeeping, not a change to the project's files).
var diagnosisGit = map[string]bool{
	"add": true, "blame": true, "branch": true, "commit": true, "diff": true, "fetch": true,
	"grep": true, "log": true, "ls-files": true, "push": true, "show": true, "stash": true,
	"status": true, "switch": true, "tag": true,
}

// gitCommand returns the git subcommand of a segment, if the segment starts with git.
func gitCommand(fields []string) (string, bool) {
	if len(fields) < 2 || fields[0] != "git" {
		return "", false
	}
	for _, f := range fields[1:] {
		if !strings.HasPrefix(f, "-") {
			return f, true
		}
	}
	return "", true
}

// findWrites reports whether find was given an action that deletes or runs a command.
func findWrites(args []string) bool {
	for _, a := range args[1:] {
		switch a {
		case "-delete", "-exec", "-execdir", "-ok", "-okdir":
			return true
		}
	}
	return false
}

// redirectsFile reports whether a segment sends output to a file (2>&1 is not a file).
func redirectsFile(seg string) bool {
	var quote rune
	for i, r := range seg {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == '>' && !strings.HasPrefix(strings.TrimSpace(seg[i+1:]), "&") {
			return true
		}
	}
	return false
}

// reads reports whether one simple command only reads.
func reads(fields []string) bool {
	if len(fields) == 0 || !readCommands[fields[0]] {
		return false
	}
	return fields[0] != "find" || !findWrites(fields)
}

// MayChangeTree reports whether a command line has a step beyond reading. Redirecting output counts as
// a change even when the command itself is a reader (echo x > file).
func MayChangeTree(cmd string) bool {
	for _, seg := range Segments(cmd) {
		f := Fields(seg)
		if len(f) == 0 {
			continue
		}
		if redirectsFile(seg) || !reads(f) {
			return true
		}
	}
	return false
}

// DiagnosisReadOnly reports whether every step only reads (or is version-control bookkeeping) and
// nothing redirects output. cd is ignored: it changes the working directory, not the tree.
func DiagnosisReadOnly(cmd string) bool {
	for _, seg := range Segments(cmd) {
		p := strings.TrimSpace(seg)
		if p == "" || strings.HasPrefix(p, "cd ") {
			continue
		}
		if redirectsFile(p) {
			return false
		}
		f := Fields(p)
		if len(f) == 0 {
			continue
		}
		if reads(f) {
			continue
		}
		if sub, ok := gitCommand(f); ok && diagnosisGit[sub] {
			continue
		}
		return false
	}
	return true
}
