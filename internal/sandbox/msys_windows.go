//go:build windows

package sandbox

// MSYS2 keeps its signal pipe, shared memory and namespace objects per installation. The sandboxed
// process runs from a private copy of the runtime, linked or copied once per Git version, so it gets
// its own objects and never collides with the user's Git Bash. The copy is content-identical to the
// installed runtime; only the installation path differs.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"infuse/internal/core"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// prepareMSYS returns the private bash for an MSYS2/bash path, building the private runtime on first
// use. It returns ("", nil) when the command is not an MSYS2 program.
func prepareMSYS(command string) (string, error) {
	if !isMSYSProgram(command) {
		return "", nil
	}
	root, ok := msysInstallRoot(command)
	if !ok {
		return "", fmt.Errorf("cannot locate the MSYS2 installation for %s", command)
	}
	sandboxDir := filepath.Join(core.InfuseHome(), "sandbox")
	if strings.HasPrefix(strings.ToLower(filepath.Clean(command)), strings.ToLower(sandboxDir)) {
		return command, nil
	}
	dll := filepath.Join(root, "usr", "bin", "msys-2.0.dll")
	st, err := os.Stat(dll)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", dll, err)
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root)) + "|" + strconv.FormatInt(st.Size(), 10) + "|" + strconv.FormatInt(st.ModTime().UnixNano(), 10)))
	dest := filepath.Join(sandboxDir, "msys-"+hex.EncodeToString(sum[:6]))
	private := filepath.Join(dest, "usr", "bin", "bash.exe")
	if _, err := os.Stat(private); err == nil {
		return private, nil
	}
	if err := buildPrivateMSYS(root, dest); err != nil {
		return "", err
	}
	pruneOldRuntimes(filepath.Dir(dest), filepath.Base(dest))
	return private, nil
}

// pruneOldRuntimes drops completed runtimes for earlier Git versions; the current one is kept and
// directories still in use by another command fail to delete harmlessly.
func pruneOldRuntimes(dir, keep string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && strings.HasPrefix(name, "msys-") && !strings.Contains(name, ".build-") && name != keep {
			_ = os.RemoveAll(filepath.Join(dir, name))
		}
	}
}

func isMSYSProgram(command string) bool {
	switch strings.ToLower(strings.TrimSuffix(filepath.Base(command), filepath.Ext(command))) {
	case "bash", "sh":
		return true
	}
	return false
}

// msysInstallRoot accepts <root>\usr\bin\bash.exe and the <root>\bin\bash.exe launcher.
func msysInstallRoot(command string) (string, bool) {
	dir := filepath.Dir(filepath.Clean(command))
	if !strings.EqualFold(filepath.Base(dir), "bin") {
		return "", false
	}
	root := filepath.Dir(dir)
	if strings.EqualFold(filepath.Base(root), "usr") {
		root = filepath.Dir(root)
	}
	if _, err := os.Stat(filepath.Join(root, "usr", "bin", "msys-2.0.dll")); err != nil {
		return "", false
	}
	return root, true
}

// buildPrivateMSYS assembles the copy beside its final name and renames it, so a concurrent command
// either sees a complete runtime or builds its own.
func buildPrivateMSYS(source, dest string) error {
	build := dest + ".build-" + strconv.Itoa(os.Getpid())
	_ = os.RemoveAll(build)
	defer os.RemoveAll(build)
	for _, rel := range []string{filepath.Join("usr", "bin"), "etc", filepath.Join("usr", "lib")} {
		src := filepath.Join(source, rel)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := syncTree(src, filepath.Join(build, rel)); err != nil {
			return err
		}
	}
	if err := os.Rename(build, dest); err != nil {
		if _, statErr := os.Stat(filepath.Join(dest, "usr", "bin", "bash.exe")); statErr == nil {
			return nil
		}
		return fmt.Errorf("install the private MSYS2 runtime: %w", err)
	}
	return nil
}

// syncTree mirrors one directory tree, preferring hard links so the private runtime costs no extra
// disk space on the same volume and falls back to a copy elsewhere.
func syncTree(src, dst string) error {
	hardlinks := true
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		source := path
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return nil
			}
			if !filepath.IsAbs(link) {
				link = filepath.Join(filepath.Dir(path), link)
			}
			st, err := os.Stat(link)
			if err != nil || st.IsDir() {
				return nil
			}
			source = link
			info = st
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(target); err == nil {
			return nil
		}
		if hardlinks {
			if err := os.Link(source, target); err == nil {
				return nil
			}
			hardlinks = false // the source volume forbids links from this install
		}
		return copyRuntimeFile(source, target, info.Mode().Perm())
	})
}

func copyRuntimeFile(source, target string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
