//go:build !windows && !linux && !darwin

package sandbox

// Platforms without an OS-level containment: commands run as before and the turn audit judges what
// changed.

func platformAvailable() bool { return false }

// RunWrapper is not reached here (Wrap keeps the argv), but the role must link on every platform.
func RunWrapper(root string, protected []string, argv []string) int {
	return runUncontained(argv)
}
