package workspace

// The dependency fingerprint a turn keeps: the walk itself is in fsx, below the oracle.

import "infuse/internal/fsx"

// DependencyFingerprint identifies the manifests and the installed code of a project.
func DependencyFingerprint(work string) string { return fsx.DependencyFingerprint(work) }
