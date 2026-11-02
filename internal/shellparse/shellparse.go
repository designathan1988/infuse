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
	"basename": true, "cat": true, "cd": true, "cmp": true, "cut": true,
	"diff": true, "dir": true, "dirname": true, "du": true, "echo": true, "file": true,
	"find": true, "grep": true, "groups": true, "head": true, "id": true, "info": true,
	"jq": true, "less": true, "ls": true, "man": true, "md5sum": true, "more": true,
	"pwd": true, "readlink": true, "realpath": true, "rg": true, "sha1sum": true, "sha256sum": true,
	"sort": true, "stat": true, "tail": true, "tr": true, "tree": true, "type": true,
	"uname": true, "uniq": true, "wc": true, "where": true, "which": true, "whoami": true,
}

// DestructiveGit recognizes commands that can erase the working tree, the index or refs without
// naming the protected paths: git clean, reset --hard, checkout -- ., restore, branch/tag deletion,
// forced push and rm.
func DestructiveGit(cmd string) bool {
	for _, seg := range Segments(cmd) {
		f := Fields(seg)
		if len(f) == 0 || f[0] != "git" {
			continue
		}
		i := 1
		for i < len(f) && strings.HasPrefix(f[i], "-") {
			if f[i] == "-C" {
				i += 2
				continue
			}
			i++
		}
		if i >= len(f) {
			continue
		}
		sub, args := f[i], f[i+1:]
		switch sub {
		case "clean", "rm":
			return true
		case "reset":
			if containsFlag(args, "--hard", "--merge", "--keep") {
				return true
			}
		case "checkout":
			if containsFlag(args, "--", "-f", "--force", ".") {
				return true
			}
		case "restore":
			return true
		case "branch", "tag":
			if containsFlag(args, "-d", "-D", "--delete") {
				return true
			}
		case "push":
			if containsFlag(args, "--force", "-f", "--force-with-lease") {
				return true
			}
		}
	}
	return false
}

// diagnosisGit are the git subcommands the diagnosis gate lets through unchanged: inspecting the
// repository, never changing the index, refs, the working tree or the remote.
var diagnosisGit = map[string]bool{
	"blame": true, "diff": true, "grep": true, "log": true, "ls-files": true,
	"rev-parse": true, "show": true, "status": true,
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
		case "-delete", "-exec", "-execdir", "-ok", "-okdir",
			"-fprint", "-fprint0", "-fprintf", "-fls":
			return true
		}
	}
	return false
}

// versionOnly recognizes a program asked only to print its version. It is read-only, but node -e,
// python -c and friends are execution and never match it.
func versionOnly(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	for _, arg := range fields[1:] {
		switch strings.ToLower(arg) {
		case "--version", "-version", "-v", "-V", "version", "--info":
		default:
			return false
		}
	}
	return true
}

func sortWrites(args []string) bool {
	for _, a := range args[1:] {
		if a == "-o" || a == "--output" || strings.HasPrefix(a, "--output=") || (strings.HasPrefix(a, "-o") && len(a) > 2) {
			return true
		}
	}
	return false
}

func commandQuery(args []string) bool {
	return len(args) >= 2 && args[0] == "command" && (args[1] == "-v" || args[1] == "-V")
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
	if len(fields) == 0 {
		return false
	}
	if versionOnly(fields) || commandQuery(fields) {
		return true
	}
	switch fields[0] {
	case "date":
		return !containsFlag(fields[1:], "-s", "--set")
	case "hostname":
		for _, arg := range fields[1:] {
			if !strings.HasPrefix(arg, "-") {
				return false // hostname <new name> changes the host
			}
		}
		return true
	}
	if !readCommands[fields[0]] {
		return false
	}
	if fields[0] == "find" {
		return !findWrites(fields)
	}
	if fields[0] == "sort" {
		return !sortWrites(fields)
	}
	return true
}

func containsFlag(args []string, flags ...string) bool {
	for _, a := range args {
		for _, f := range flags {
			if a == f {
				return true
			}
		}
	}
	return false
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
