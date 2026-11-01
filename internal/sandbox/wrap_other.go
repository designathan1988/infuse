//go:build !linux && !windows

package sandbox

// macOS has no supported unprivileged equivalent here; the command runs unchanged. Windows uses the
// WRITE_RESTRICTED token wrapper in wrapper_windows.go.

func Wrap(argv []string, _ string, _ []string) []string { return argv }
