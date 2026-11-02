//go:build !windows

package sandbox

// PrepareRoot has nothing to do without mandatory labels: Landlock and Seatbelt take the root as given.
func PrepareRoot(path string) error { return nil }
