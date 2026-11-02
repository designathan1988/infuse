package filter

// Commands that act on a whole folder: removing, moving or overwriting a folder takes every protected
// path inside it along, although none of them is named (rm -rf ., mv .infuse x, cp -r evil .infuse,
// git clean -fdx). A plain copy INTO the project root stays allowed: `cp a.txt .` is everyday work.

import (
	"infuse/internal/core"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	treeRemovers = map[string]bool{"rm": true, "rmdir": true, "del": true, "erase": true, "rd": true, "remove-item": true, "ri": true}
	treeMovers   = map[string]bool{"mv": true, "move": true, "move-item": true, "mi": true, "ren": true, "rename": true, "rename-item": true, "rni": true}
	treeCopiers  = map[string]bool{"cp": true, "copy": true, "xcopy": true, "robocopy": true, "copy-item": true, "cpi": true, "rsync": true, "install": true}

	gitWriting = map[string]bool{"rm": true, "clean": true, "stash": true, "checkout": true, "restore": true, "reset": true, "mv": true, "apply": true}
	// gitValueFlags are git's global options that take a separate value.
	gitValueFlags = map[string]bool{"-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true}
	// gitConfigEdits are the `git config` options that change the file even with one positional.
	gitConfigEdits = map[string]bool{"--add": true, "--unset": true, "--unset-all": true, "--replace-all": true, "--rename-section": true, "--remove-section": true, "set": true, "unset": true}

	reCmdSwitch = regexp.MustCompile(`^/[A-Za-z?]{1,4}(:.*)?$`) // /s, /E, /MIR, /XF:… of cmd.exe tools
)

// pathArgs keeps the positional arguments of a command.
func pathArgs(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") && !reCmdSwitch.MatchString(a) {
			out = append(out, a)
		}
	}
	return out
}

// treeEffect splits a command's paths into the folders it removes, where it writes and what it copies.
func treeEffect(head string, paths []string) (removed []string, dest string, copied []string) {
	n := len(paths)
	switch {
	case treeRemovers[head]:
		return paths, "", nil
	case treeMovers[head] && n > 0:
		return paths[:n-1], paths[n-1], paths[:n-1]
	case treeCopiers[head] && head == "robocopy" && n > 1:
		return nil, paths[1], paths[:1]
	case treeCopiers[head] && n > 1:
		return nil, paths[n-1], paths[:n-1]
	}
	return nil, "", nil
}

// checkTree refuses a command that removes or overwrites a folder holding a protected path.
func (g *Guard) checkTree(head string, args []string, dir, text string) error {
	removed, dest, copied := treeEffect(head, pathArgs(args))
	for _, p := range removed {
		if rel, ok := g.holdsProtected(dir, p); ok {
			return g.block(rel, core.M("guard.with_command", "why", core.M("guard.tree"), "command", core.Truncate(strings.TrimSpace(text), 120)))
		}
	}
	if dest == "" {
		return nil
	}
	rel, ok := g.overwrites(dir, dest, copied)
	if ok {
		return g.block(rel, core.M("guard.with_command", "why", core.M("guard.tree"), "command", core.Truncate(strings.TrimSpace(text), 120)))
	}
	return nil
}

// overwrites reports whether copying into dest reaches a protected path: dest is a folder holding one
// (the project root itself excepted), or a source lands on one by its own name.
func (g *Guard) overwrites(dir, dest string, sources []string) (string, bool) {
	if rel, ok := g.holdsProtected(dir, dest); ok && !g.isRootOrAbove(dir, dest) {
		return rel, true
	}
	for _, s := range sources {
		name := filepath.Base(filepath.FromSlash(strings.ReplaceAll(s, `\`, "/")))
		if rel, ok := g.Protects(dir, filepath.Join(CanonPath(dir, dest), name)); ok {
			return rel, true
		}
	}
	return "", false
}

func (g *Guard) isRootOrAbove(dir, p string) bool {
	abs := RealPath(CanonPath(dir, p))
	for _, root := range g.roots {
		if core.Within(abs, root) {
			return true
		}
	}
	return false
}

// holdsProtected reports whether p is a project root, a folder above one, or a folder that holds a
// protected path.
func (g *Guard) holdsProtected(dir, p string) (string, bool) {
	p = strings.Trim(p, `"'`)
	if p == "" || isNullDevice(p) {
		return "", false
	}
	for _, abs := range pathSpellings(CanonPath(dir, p)) {
		for _, root := range g.roots {
			if core.Within(abs, root) {
				return core.FirstNonEmpty(core.RelTo(root, abs), "."), true
			}
			if core.Within(root, abs) && g.holdsPattern(foldPath(core.RelTo(root, abs))) {
				return core.RelTo(root, abs), true
			}
		}
	}
	return "", false
}

func (g *Guard) holdsPattern(rel string) bool {
	for _, pat := range g.Protected {
		if strings.HasPrefix(foldPath(pat), rel+"/") {
			return true
		}
	}
	return false
}

// gitCommandLine finds git's subcommand past its global options, following -C.
func gitCommandLine(args []string, dir string) (string, []string, string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-C" && i+1 < len(args):
			i++
			dir = resolveDir(dir, args[i])
		case gitValueFlags[a]:
			i++
		case !strings.HasPrefix(a, "-"):
			return a, args[i+1:], dir
		}
	}
	return "", nil, dir
}

// checkGit applies the policy of the git subcommands that change files or the repository's config.
func (g *Guard) checkGit(args []string, dir, text string) error {
	sub, rest, dir := gitCommandLine(args, dir)
	switch {
	case sub == "clean":
		if err := g.checkGitClean(rest, dir); err != nil {
			return err
		}
	case sub == "config" && gitConfigWrites(rest):
		return g.block(".git/config", core.M("guard.git", "sub", sub))
	case sub == "rm" || sub == "mv":
		if err := g.checkTree(sub, rest, dir, text); err != nil {
			return err
		}
	}
	if gitWriting[sub] {
		return g.refuseArgs(rest, dir, text, core.M("guard.git", "sub", sub))
	}
	return nil
}

// checkGitClean refuses a real clean that is not confined to named paths: it removes what is untracked
// or ignored, and that is where infuse's records and the oracle usually are.
func (g *Guard) checkGitClean(args []string, dir string) error {
	var paths []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--dry-run" || strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") && strings.Contains(a, "n"):
			return nil
		case a == "-e" || a == "--exclude":
			i++
		case !strings.HasPrefix(a, "-"):
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		return g.block(".", core.M("guard.clean"))
	}
	for _, p := range paths {
		if rel, ok := g.holdsProtected(dir, p); ok {
			return g.block(rel, core.M("guard.clean"))
		}
	}
	return nil
}

// gitConfigWrites reports whether `git config <args>` changes the repository's config file.
func gitConfigWrites(args []string) bool {
	positional := 0
	for _, a := range args {
		switch {
		case a == "--global" || a == "--system" || a == "get" || a == "list" || strings.HasPrefix(a, "--get") || a == "--list" || a == "-l":
			return false
		case gitConfigEdits[a]:
			return true
		case !strings.HasPrefix(a, "-"):
			positional++
		}
	}
	return positional > 1
}
