// Package gitx is the only place infuse starts git. A repository's own configuration can name programs
// (hooks, an fsmonitor, an ssh command, clean and smudge filters, credential helpers), and the agent
// shapes the repository, so a host-side git must never run them: it would execute project-controlled
// code at the user's full rights. Every invocation carries overrides that neutralize those settings.
//
// Filter, diff and merge drivers have no global off switch in git: they are looked up by name, so the
// names the repository's own configuration defines are read first and replaced one by one. Drivers from
// the user's global configuration stay (git-lfs lives there): they are the user's, not the project's.
package gitx

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"

	"infuse/internal/platform/process"
)

// overrides neutralize every setting that makes git run a program named by the repository.
var overrides = []string{
	"core.fsmonitor=false",
	"core.hooksPath=" + os.DevNull,
	"core.sshCommand=ssh",
	"core.gitProxy=",
	"core.askPass=",
	"core.alternateRefsCommand=",
	"core.pager=cat",
	"core.editor=true",
	"sequence.editor=true",
	"gpg.program=gpg",
	"gpg.ssh.program=ssh-keygen",
	"gpg.x509.program=gpgsm",
	"protocol.ext.allow=never",
}

// driverKey matches the configuration keys that name a program under a driver of any name.
var driverKey = regexp.MustCompile(`(?i)^((filter\..+\.(clean|smudge|process))|(diff\.external)|(diff\..+\.(command|textconv))|(merge\..+\.driver))$`)

// Overrides returns the -c arguments for dir: the fixed set plus the repository's own drivers.
func Overrides(dir string) []string {
	var args []string
	for _, o := range overrides {
		args = append(args, "-c", o)
	}
	for _, h := range userCredentialHelpers() {
		args = append(args, "-c", "credential.helper="+h)
	}
	for _, key := range repositoryDrivers(dir) {
		args = append(args, "-c", key+"="+inertDriver(key))
	}
	return args
}

// inertDriver is the harmless stand-in for a driver: an empty filter means none, but git would try to
// spawn an empty diff or merge driver, so those get a command that only git's own shell runs.
func inertDriver(key string) string {
	switch strings.ToLower(key[strings.LastIndex(key, ".")+1:]) {
	case "textconv":
		return "cat"
	case "command", "external":
		return "true"
	case "driver":
		return "false"
	}
	return ""
}

// Command builds a git invocation in dir carrying the overrides. Nothing else may exec git.
func Command(ctx context.Context, dir string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append(Overrides(dir), args...)...)
	process.NoConsoleWindow(cmd)
	cmd.Dir = dir
	return cmd
}

// Run runs git in dir and returns its combined output.
func Run(dir string, args ...string) (string, error) {
	out, err := Command(context.Background(), dir, args...).CombinedOutput()
	return string(out), err
}

// Output runs git in dir and returns its trimmed stdout ("" on failure).
func Output(dir string, args ...string) string {
	out, err := Command(context.Background(), dir, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Env is the same fixed set as environment variables, for programs that start git themselves (gh).
func Env() []string {
	env := os.Environ()
	all := append(append([]string{}, overrides...), credentialValues()...)
	env = append(env, "GIT_CONFIG_COUNT="+itoa(len(all)))
	for i, kv := range all {
		k, v, _ := strings.Cut(kv, "=")
		env = append(env, "GIT_CONFIG_KEY_"+itoa(i)+"="+k, "GIT_CONFIG_VALUE_"+itoa(i)+"="+v)
	}
	return env
}

func credentialValues() []string {
	var out []string
	for _, h := range userCredentialHelpers() {
		out = append(out, "credential.helper="+h)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for ; n > 0; n /= 10 {
		s = string(rune('0'+n%10)) + s
	}
	return s
}

// repositoryDrivers lists the driver keys set by the repository's own configuration files. Reading
// configuration runs nothing, and this listing is what the -c arguments then cancel.
func repositoryDrivers(dir string) []string {
	cmd := exec.Command("git", "config", "--list", "--show-scope", "--name-only", "--includes", "-z")
	process.NoConsoleWindow(cmd)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	fields := strings.Split(string(out), "\x00")
	var keys []string
	for i := 0; i+1 < len(fields); i += 2 {
		scope, key := fields[i], strings.TrimSpace(fields[i+1])
		if (scope == "local" || scope == "worktree" || scope == "command") && driverKey.MatchString(key) {
			keys = append(keys, key)
		}
	}
	return keys
}

var (
	helpersOnce sync.Once
	helpers     []string
)

// userCredentialHelpers keeps the system and user helpers and clears the repository's: an empty value
// resets git's list, and the names the user configured are set again after it. Read from the user's
// files only, outside any repository, so a project's configuration cannot add to the list.
func userCredentialHelpers() []string {
	helpersOnce.Do(func() {
		helpers = []string{""}
		for _, scope := range []string{"--system", "--global"} {
			cmd := exec.Command("git", "config", scope, "--get-all", "credential.helper")
			process.NoConsoleWindow(cmd)
			cmd.Dir = os.TempDir()
			out, err := cmd.Output()
			if err != nil {
				continue
			}
			for _, h := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if h = strings.TrimSpace(h); h != "" {
					helpers = append(helpers, h)
				}
			}
		}
	})
	return helpers
}
