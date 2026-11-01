//go:build !linux

package sandbox

// Windows containment is the Job Object created for every agent process (lifetime) plus the filter's
// path gates; a full deny-write OS sandbox is blocked by the MSYS2 runtime (see docs/AUDIT-REPORT.md).
// macOS has no supported unprivileged equivalent here. The command therefore runs unchanged.

func Wrap(argv []string, _ string, _ []string) []string { return argv }
