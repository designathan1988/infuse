package filter

// One name per file. A path check is only as good as the spelling it compares, and Windows accepts many
// spellings of one file: every gate (the guard, the tools, the heredoc transform, the rules) turns what
// the agent wrote into the name the filesystem means before it looks at it.
//   - trailing dots and spaces on a component are dropped by Win32 (.mcp.json. is .mcp.json)
//   - a stream suffix names the file or folder it belongs to (x::$DATA, dir::$INDEX_ALLOCATION, x:name)
//   - \\?\C:\x, \\.\C:\x and \??\C:\x are C:\x; \\?\UNC\h\s and \\h\s are the same share
//   - \\localhost\c$\x is C:\x; any other share or device stays what it is: outside the project
//   - C:x is relative to the current folder of drive C, and /x is rooted on the drive of the cwd
//   - 8.3 short names (INFUSE~1) are expanded by RealPath, which reads them from the disk
//
// Elsewhere only the working directory and ~ matter: a trailing dot is part of a Unix file name.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	reDrive         = regexp.MustCompile(`^[A-Za-z]:`)
	reDriveRelative = regexp.MustCompile(`^[A-Za-z]:($|[^/])`)
	reAdminShare    = regexp.MustCompile(`(?i)^//([^/]+)/([a-z])\$(/.*)?$`)
)

// maxLinkHops bounds a chain of links that do not resolve; a loop must not hang a path check.
const maxLinkHops = 32

// CanonPath is the absolute, clean, canonical spelling of p as seen from cwd. Symlinks are not followed:
// RealPath does that, and a check looks at both.
func CanonPath(cwd, p string) string {
	p = expandHome(strings.ReplaceAll(p, `\`, "/"))
	if runtime.GOOS == "windows" {
		return canonWindows(cwd, p)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Clean(p)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.ToSlash(core.HomeDir()) + p[1:]
	}
	return p
}

func canonWindows(cwd, p string) string {
	p = strings.ReplaceAll(core.FromMsysPath(loopbackShare(stripDevicePrefix(p))), `\`, "/")
	p = anchorWindows(cwd, p)
	vol := filepath.VolumeName(p)
	parts := strings.Split(p[len(vol):], "/")
	for i, part := range parts {
		parts[i] = canonComponent(part)
	}
	return filepath.Clean(filepath.FromSlash(vol + strings.Join(parts, "/")))
}

// stripDevicePrefix removes the prefixes that only skip Win32 normalization. A device that is not a
// drive (\\?\GLOBALROOT, \\?\Volume{…}, \\.\pipe) keeps its name: it is not a path inside the project.
func stripDevicePrefix(p string) string {
	for _, prefix := range []string{"//?/", "//./", "/??/"} {
		if !strings.HasPrefix(p, prefix) {
			continue
		}
		rest := p[len(prefix):]
		if reDrive.MatchString(rest) {
			return rest
		}
		if len(rest) >= 4 && strings.EqualFold(rest[:4], "UNC/") {
			return "//" + rest[4:]
		}
	}
	return p
}

// loopbackShare turns the administrative share of this machine into the drive it exposes.
func loopbackShare(p string) string {
	m := reAdminShare.FindStringSubmatch(p)
	if m == nil || !isLoopback(m[1]) {
		return p
	}
	return m[2] + ":" + core.FirstNonEmpty(m[3], "/")
}

func isLoopback(host string) bool {
	host = strings.ToLower(host)
	if host == "localhost" || host == "::1" || host == "[::1]" || host == "." || strings.HasPrefix(host, "127.") {
		return true
	}
	name, err := os.Hostname()
	return err == nil && strings.EqualFold(name, host)
}

// anchorWindows makes p absolute. Its input and output use forward slashes.
func anchorWindows(cwd, p string) string {
	switch {
	case reDriveRelative.MatchString(p):
		if len(cwd) >= 2 && strings.EqualFold(cwd[:2], p[:2]) {
			return filepath.ToSlash(filepath.Join(cwd, p[2:]))
		}
		return p[:2] + "/" + p[2:]
	case filepath.IsAbs(p):
		return p
	case strings.HasPrefix(p, "/"):
		return filepath.ToSlash(filepath.VolumeName(cwd)) + p
	}
	return filepath.ToSlash(filepath.Join(cwd, p))
}

// canonComponent drops a stream suffix and the trailing dots and spaces of one path component.
func canonComponent(part string) string {
	if i := strings.IndexByte(part, ':'); i >= 0 {
		part = part[:i]
	}
	kept := strings.TrimRight(part, " ")
	if kept == "." || kept == ".." {
		return kept
	}
	kept = strings.TrimRight(kept, ". ")
	if kept == "" && part != "" {
		return "." // a component of dots and spaces alone names nothing
	}
	return kept
}

// RealPath follows links through the deepest existing ancestor of abs, so a path that does not exist yet
// is judged where it would land. A link whose target does not exist is followed too.
func RealPath(abs string) string { return realPath(abs, 0) }

func realPath(abs string, hops int) string {
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		return r
	}
	tail := ""
	for cur := abs; hops < maxLinkHops; {
		if target, err := os.Readlink(cur); err == nil {
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(cur), target)
			}
			return realPath(filepath.Join(target, tail), hops+1)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return abs
		}
		tail = filepath.Join(filepath.Base(cur), tail)
		cur = parent
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(r, tail)
		}
	}
	return abs
}

// pathSpellings lists what abs can mean on disk: itself, what a shell wildcard in it expands to, and
// where each of those really is.
func pathSpellings(abs string) []string {
	out := []string{abs}
	if strings.ContainsAny(abs, "*?[") {
		if matches, err := filepath.Glob(abs); err == nil {
			out = append(out, matches...)
		}
	}
	for _, p := range out[:len(out):len(out)] {
		if real := RealPath(p); real != p {
			out = append(out, real)
		}
	}
	return out
}
