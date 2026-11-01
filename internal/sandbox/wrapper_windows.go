//go:build windows

package sandbox

// The Windows sandbox runs the command through this binary's hidden wrapper mode. The wrapper keeps
// the caller's reads, grants write access only to the work root and a private temp directory, and
// starts the command with a WRITE_RESTRICTED token, which the whole process tree inherits.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/core"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func Wrap(argv []string, root string, protected []string) []string {
	if len(argv) == 0 || root == "" || testBinary() {
		return argv
	}
	self, err := os.Executable()
	if err != nil {
		return argv
	}
	b, _ := json.Marshal(protected)
	return append([]string{self, "__sandbox", root, string(b), "--"}, argv...)
}

// RunWrapper applies the Windows sandbox and runs the requested command under it.
func RunWrapper(args []string) int {
	if len(args) < 4 || args[2] != "--" {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox: malformed wrapper arguments")
		return 126
	}
	var protected []string
	if json.Unmarshal([]byte(args[1]), &protected) != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox: malformed protected-path list")
		return 126
	}
	command := args[3:]
	if os.Getenv("INFUSE_SANDBOX_DEBUG") != "" {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox PATH in:", os.Getenv("PATH"))
	}
	private, err := prepareMSYS(command[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox:", err)
		return 126
	}
	extraPath := ""
	if private != "" {
		command[0] = private
		extraPath = filepath.Dir(private)
	}
	code, err := runConfined(args[0], protected, command, extraPath, os.Stdin, os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "[infuse] sandbox:", err)
		return 126
	}
	return code
}

// runConfined is the testable core of the wrapper: it sets up the token and ACLs and waits for the
// command. It fails closed when the token or an ACL cannot be applied.
func runConfined(root string, protected []string, command []string, extraPath string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(command) == 0 {
		return 126, errors.New("no command to run")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return 126, err
	}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return 126, fmt.Errorf("work root %q is not a directory", root)
	}
	rt, err := newRestrictedToken(root)
	if err != nil {
		return 126, err
	}
	defer rt.close()
	if err := prepareWorkspace(root, protected, rt.capSID); err != nil {
		return 126, err
	}
	temp, err := privateTemp(rt.capSID)
	if err != nil {
		return 126, err
	}
	defer os.RemoveAll(temp)
	cache, err := privateCache(rt.capSID)
	if err != nil {
		return 126, err
	}

	path, err := exec.LookPath(command[0])
	if err != nil {
		return 126, err
	}
	cmd := exec.Command(path, command[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	cmd.Env = sandboxEnv(temp, cache, extraPath)
	// HideWindow only sets STARTF_USESHOWWINDOW/SW_HIDE; CREATE_NO_WINDOW breaks DLL initialization
	// under a Low token, so it is deliberately not used here.
	cmd.SysProcAttr = &syscall.SysProcAttr{Token: syscall.Token(rt.token), HideWindow: true}
	if err := cmd.Start(); err != nil {
		return 126, fmt.Errorf("start %s: %w", command[0], err)
	}
	err = cmd.Wait()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), nil
	}
	if err != nil {
		return 126, err
	}
	return 0, nil
}

// prepareWorkspace grants write to the capability SID, lowers the tree's integrity label so the
// confined token can write existing files, and raises .infuse and the protected paths back to Medium
// (protected, so the inherited Low label cannot apply there).
func prepareWorkspace(root string, protected []string, sid *windows.SID) error {
	low, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		return err
	}
	if !grantIsStanding(root, sid) {
		if err := grantWrite(root, sid); err != nil {
			return err
		}
		if err := setLabel(root, low, true, false); err != nil {
			return err
		}
		writeGrantMarker(root, sid)
	}
	medium, err := windows.CreateWellKnownSid(windows.WinMediumLabelSid)
	if err != nil {
		return err
	}
	for _, path := range protectedPaths(root, protected) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		if err := setLabel(path, medium, info.IsDir(), false); err != nil {
			return err
		}
	}
	return nil
}

// The capability ACE and the Low label are inheritable and standing; a marker keyed by the work root
// avoids re-propagating them through the whole tree on every command.
func grantMarker(root string, sid *windows.SID) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root)) + "|" + sid.String()))
	return filepath.Join(core.InfuseHome(), "sandbox", "grants", hex.EncodeToString(sum[:8])+".grant")
}

func grantIsStanding(root string, sid *windows.SID) bool {
	b, err := os.ReadFile(grantMarker(root, sid))
	return err == nil && strings.TrimSpace(string(b)) == sid.String()
}

func writeGrantMarker(root string, sid *windows.SID) {
	path := grantMarker(root, sid)
	if os.MkdirAll(filepath.Dir(path), 0o700) != nil {
		return
	}
	_ = os.WriteFile(path, []byte(sid.String()), 0o644)
}

// protectedPaths lists the records and user files inside the root that the confined process must not
// reach even though the root grants write.
func protectedPaths(root string, protected []string) []string {
	candidates := []string{filepath.Join(root, core.InfuseDirName)}
	for _, rel := range protected {
		rel = strings.TrimSpace(strings.TrimSuffix(filepath.ToSlash(rel), "/**"))
		if rel == "" || filepath.IsAbs(rel) || strings.ContainsAny(rel, "*?[") {
			continue
		}
		if rel == core.InfuseDirName || strings.HasPrefix(rel, core.InfuseDirName+"/") {
			continue
		}
		candidates = append(candidates, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return candidates
}

func privateTemp(sid *windows.SID) (string, error) {
	base := filepath.Join(core.InfuseHome(), "sandbox")
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(base, "win-")
	if err != nil {
		return "", err
	}
	if err := ensureGrant(dir, sid); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// privateCache is the only writable place outside the work root: toolchain caches (Go modules,
// npm/pip/playwright) are redirected here so ordinary builds keep working without opening the user's
// profile. It is persistent and granted once.
func privateCache(sid *windows.SID) (string, error) {
	dir := filepath.Join(core.InfuseHome(), "sandbox", "cache")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if err := ensureGrant(dir, sid); err != nil {
		return "", err
	}
	return dir, nil
}

// ensureGrant applies the capability ACE and the Low label to a directory unless the standing marker
// says they are already there.
func ensureGrant(dir string, sid *windows.SID) error {
	if grantIsStanding(dir, sid) {
		return nil
	}
	low, err := windows.CreateWellKnownSid(windows.WinLowLabelSid)
	if err != nil {
		return err
	}
	if err := grantWrite(dir, sid); err != nil {
		return err
	}
	if err := setLabel(dir, low, true, false); err != nil {
		return err
	}
	writeGrantMarker(dir, sid)
	return nil
}

// sandboxEnv redirects temp files into the private temp directory, caches into the granted cache
// directory, and (when prepared) puts the private MSYS2 bin first on PATH. Duplicate entries are
// removed because Windows resolves them unpredictably.
func sandboxEnv(temp, cache, extraPath string) []string {
	overrides := cacheOverrides(temp, cache)
	drop := map[string]bool{"TMP": true, "TEMP": true, "TMPDIR": true}
	for _, kv := range overrides {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			drop[strings.ToUpper(kv[:i])] = true
		}
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+len(overrides))
	for _, kv := range env {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if drop[strings.ToUpper(key)] {
			continue
		}
		if strings.EqualFold(key, "PATH") {
			value := normalizePath(kv[len(key)+1:])
			if extraPath != "" {
				value = extraPath + ";" + value
			}
			kv = key + "=" + value
		}
		out = append(out, kv)
	}
	return append(out, overrides...)
}

// normalizePath converts a POSIX-style PATH (as Git Bash passes to a native child) into the
// semicolon-separated Windows form MSYS2 expects; leaving it mixed corrupts the first entry. POSIX
// system directories are dropped because the private runtime supplies its own.
func normalizePath(value string) string {
	if !strings.Contains(value, ";") {
		parts := splitPOSIXPath(value)
		out := make([]string, 0, len(parts))
		for _, part := range parts {
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

func windowsPath(part string) (string, bool) {
	part = strings.TrimSpace(part)
	if part == "" {
		return "", false
	}
	if len(part) >= 3 && part[0] == '/' && isDriveLetter(part[1]) && part[2] == '/' {
		return strings.ToUpper(part[1:2]) + ":\\" + strings.ReplaceAll(part[3:], "/", "\\"), true
	}
	if len(part) >= 3 && part[1] == ':' {
		return strings.ReplaceAll(part, "/", "\\"), true
	}
	low := strings.ToLower(part)
	for _, prefix := range []string{"/usr", "/bin", "/mingw64", "/ucrt64", "/clang64"} {
		if low == prefix || strings.HasPrefix(low, prefix+"/") {
			return "", false
		}
	}
	return "", false
}

// cacheOverrides keeps builds working without granting write to the user's profile: tool caches are
// redirected to the granted sandbox cache, while configuration files stay readable in their real home.
func cacheOverrides(temp, cache string) []string {
	join := func(parts ...string) string { return filepath.Join(append([]string{cache}, parts...)...) }
	return []string{
		"TMP=" + temp, "TEMP=" + temp, "TMPDIR=" + temp,
		"GOCACHE=" + join("go-build"),
		"GOMODCACHE=" + join("go-mod"),
		"GOTMPDIR=" + temp,
		"NPM_CONFIG_CACHE=" + join("npm"),
		"YARN_CACHE_FOLDER=" + join("yarn"),
		"PNPM_STORE_DIR=" + join("pnpm-store"),
		"PIP_CACHE_DIR=" + join("pip"),
		"XDG_CACHE_HOME=" + join("xdg"),
		"PLAYWRIGHT_BROWSERS_PATH=" + join("ms-playwright"),
		"CYPRESS_CACHE_FOLDER=" + join("cypress"),
	}
}
