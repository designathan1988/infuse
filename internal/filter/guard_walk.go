package filter

// The working directory across a command. The guard and the heredoc transform must agree on where a
// relative path lands, so both follow it with this one walker.

import (
	"infuse/internal/core"
	"strings"
)

type dirWalk struct {
	dir, prev string
	pushed    []string // pushd
	scopes    []string // the directory to return to when a ( subshell ) closes
}

func newDirWalk(cwd string) *dirWalk { return &dirWalk{dir: cwd, prev: cwd} }

// step moves past one segment.
func (w *dirWalk) step(sg shellSeg) {
	if fields := commandWords(shellFields(sg.text)); len(fields) > 0 {
		w.follow(fields)
	}
	switch sg.sep {
	case "(":
		w.scopes = append(w.scopes, w.dir)
	case ")":
		if n := len(w.scopes); n > 0 {
			w.dir, w.scopes = w.scopes[n-1], w.scopes[:n-1]
		}
	}
}

// follow applies cd / pushd / popd.
func (w *dirWalk) follow(fields []string) {
	switch baseCmd(fields[0]) {
	case "cd", "chdir", "set-location", "sl":
		w.cd(firstArg(fields))
	case "pushd", "push-location":
		if t := firstArg(fields); t != "" {
			w.pushed = append(w.pushed, w.dir)
			w.dir = resolveDir(w.dir, t)
		}
	case "popd", "pop-location":
		if n := len(w.pushed); n > 0 {
			w.dir, w.pushed = w.pushed[n-1], w.pushed[:n-1]
		}
	}
}

func (w *dirWalk) cd(to string) {
	switch to {
	case "":
		w.dir, w.prev = core.HomeDir(), w.dir
	case "-":
		w.dir, w.prev = w.prev, w.dir
	default:
		w.dir, w.prev = resolveDir(w.dir, to), w.dir
	}
}

// firstArg is the first operand; a lone - is one (cd -).
func firstArg(fields []string) string {
	for _, f := range fields[1:] {
		if f == "-" || !strings.HasPrefix(f, "-") {
			return f
		}
	}
	return ""
}

// heredocDirs gives the directory in effect at each heredoc of cmd.
func heredocDirs(cmd, cwd string, hs []Heredoc) []string {
	out := make([]string, len(hs))
	for i := range out {
		out[i] = cwd
	}
	walk := newDirWalk(cwd)
	for _, sg := range splitSegments(maskHeredocBodies(cmd, hs)) {
		for i, h := range hs {
			if h.OpStart >= sg.start && h.OpStart < sg.end {
				out[i] = walk.dir
			}
		}
		walk.step(sg)
	}
	return out
}
