//go:build windows

package sandbox

// The MSYS runtime keeps per-user shared objects in a namespace derived from where msys-2.0.dll lives.
// When the user's own Git Bash is open those objects are Medium, and a Low process cannot map them, so
// bash dies at startup. A private copy of the runtime has its own namespace, created at Low by the
// first contained shell. It is read-only for the contained processes: it lives outside every project.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// containedShell swaps a Git-for-Windows bash for the private runtime's copy and returns the variables
// that make the swap transparent. Any other program runs as given.
func containedShell(argv []string) ([]string, [][2]string, error) {
	git := gitRootOf(argv[0])
	if git == "" {
		return argv, nil, nil
	}
	private, err := ensureRuntime(git)
	if err != nil {
		return nil, nil, fmt.Errorf("private shell runtime: %w", err)
	}
	bin := filepath.Join(private, "usr", "bin")
	out := append([]string{filepath.Join(bin, "bash.exe")}, argv[1:]...)
	path := strings.Join([]string{bin, filepath.Join(git, "mingw64", "bin"), filepath.Join(git, "cmd"), normalizePath(os.Getenv("PATH"))}, string(os.PathListSeparator))
	return out, [][2]string{{"PATH", path}}, nil
}

// gitRootOf returns the Git installation a bash path belongs to, or "".
func gitRootOf(exe string) string {
	base := strings.ToLower(filepath.Base(exe))
	if base != "bash.exe" && base != "bash" && base != "sh.exe" {
		return ""
	}
	dir := filepath.Dir(exe)
	for _, up := range []string{filepath.Dir(dir), filepath.Dir(filepath.Dir(dir))} {
		if _, err := os.Stat(filepath.Join(up, "usr", "bin", "msys-2.0.dll")); err == nil {
			return up
		}
	}
	return ""
}

// ensureRuntime copies usr/bin and etc once per Git version into the user's infuse folder. The copy is
// built aside and renamed, so a half-made runtime is never used and two first commands cannot collide.
func ensureRuntime(git string) (string, error) {
	st, err := os.Stat(filepath.Join(git, "usr", "bin", "msys-2.0.dll"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(fmt.Sprint(strings.ToLower(git), st.Size(), st.ModTime().UnixNano())))
	home, err := infuseHome()
	if err != nil {
		return "", err
	}
	dest := filepath.Join(home, "sandbox", "msys", hex.EncodeToString(sum[:6]))
	if _, err := os.Stat(filepath.Join(dest, "usr", "bin", "bash.exe")); err == nil {
		return dest, nil
	}
	tmp := fmt.Sprintf("%s.%d", dest, os.Getpid())
	defer os.RemoveAll(tmp)
	for _, rel := range []string{filepath.Join("usr", "bin"), "etc", filepath.Join("usr", "lib")} {
		if _, err := os.Stat(filepath.Join(git, rel)); err != nil {
			continue
		}
		if err := copyTree(filepath.Join(git, rel), filepath.Join(tmp, rel)); err != nil {
			return "", err
		}
	}
	if err := os.Rename(tmp, dest); err != nil && !errors.Is(err, os.ErrExist) {
		if _, statErr := os.Stat(filepath.Join(dest, "usr", "bin", "bash.exe")); statErr != nil {
			return "", err
		}
	}
	pruneOldRuntimes(filepath.Dir(dest), filepath.Base(dest))
	return dest, nil
}

// pruneOldRuntimes drops the completed runtimes of earlier Git versions. One still in use by another
// command fails to delete, harmlessly.
func pruneOldRuntimes(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && e.Name() != keep && !strings.Contains(e.Name(), ".") {
			_ = os.RemoveAll(filepath.Join(dir, e.Name()))
		}
	}
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 { // the runtime must not point back at the installation
			st, err := os.Stat(p)
			if err != nil || st.IsDir() {
				return nil
			}
			return copyFile(p, target)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return copyFile(p, target)
	})
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// normalizePath converts a POSIX-style PATH (as Git Bash passes to a native child) into the
// semicolon-separated form the MSYS runtime expects; left mixed, it corrupts the first entry. POSIX
// system folders are dropped, because the private runtime supplies its own.
func normalizePath(value string) string {
	if !strings.Contains(value, ";") {
		var out []string
		for _, part := range splitPOSIXPath(value) {
			if win, ok := windowsPath(part); ok {
				out = append(out, win)
			}
		}
		if len(out) > 0 {
			return strings.Join(out, ";")
		}
	}
	parts := strings.Split(value, ";")
	for i, part := range parts {
		if win, ok := windowsPath(part); ok {
			parts[i] = win
		}
	}
	return strings.Join(parts, ";")
}

// splitPOSIXPath splits on ':' but keeps drive prefixes such as C:/ in one piece.
func splitPOSIXPath(value string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] != ':' {
			continue
		}
		if i == start+1 && isDriveLetter(value[start]) && (i+1 == len(value) || value[i+1] == '/' || value[i+1] == '\\') {
			continue
		}
		parts = append(parts, value[start:i])
		start = i + 1
	}
	return append(parts, value[start:])
}

func isDriveLetter(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }

// windowsPath reads /c/x and C:/x as a Windows path; anything else has no Windows meaning here.
func windowsPath(part string) (string, bool) {
	part = strings.TrimSpace(part)
	switch {
	case len(part) >= 3 && part[0] == '/' && isDriveLetter(part[1]) && part[2] == '/':
		return strings.ToUpper(part[1:2]) + ":\\" + strings.ReplaceAll(part[3:], "/", "\\"), true
	case len(part) >= 3 && part[1] == ':':
		return strings.ReplaceAll(part, "/", "\\"), true
	}
	return "", false
}
