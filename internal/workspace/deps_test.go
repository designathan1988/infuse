package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// A lockfile or an installed runner changing changes the fingerprint the oracle compares.
func TestDependencyFingerprintFollowsLockfilesAndEntrypoints(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	runner := filepath.Join(bin, "vitest")
	if err := os.WriteFile(runner, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := DependencyFingerprint(root)
	if base == "" {
		t.Fatal("empty fingerprint")
	}
	if again := DependencyFingerprint(root); again != base {
		t.Fatal("fingerprint is not stable")
	}
	if err := os.WriteFile(runner, []byte("#!/bin/sh\necho tampered\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if DependencyFingerprint(root) == base {
		t.Fatal("a changed runner did not change the fingerprint")
	}
}
