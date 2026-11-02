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

// Patching a library the tests import is a dependency change even when the lockfile and .bin stay the same.
func TestDependencyFingerprintCoversInstalledLibraries(t *testing.T) {
	root := t.TempDir()
	lib := filepath.Join(root, "node_modules", "jest-runner")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(lib, "index.js")
	if err := os.WriteFile(file, []byte("module.exports = () => 0;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := DependencyFingerprint(root)
	if err := os.WriteFile(file, []byte("module.exports = () => 0; // patched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if DependencyFingerprint(root) == base {
		t.Fatal("a patched installed library did not change the fingerprint")
	}
}

// The turn manifest keeps the start fingerprint and can rebase it after an approved install.
func TestTurnDependenciesCanBeRebased(t *testing.T) {
	root := t.TempDir()
	if err := SnapshotTurn(root, root, "s1", 1); err != nil {
		t.Fatal(err)
	}
	start := TurnDependencies(root, "s1", 1)
	lib := filepath.Join(root, "node_modules", "x")
	_ = os.MkdirAll(lib, 0o755)
	_ = os.WriteFile(filepath.Join(lib, "index.js"), []byte("x\n"), 0o644)
	if start == DependencyFingerprint(root) {
		t.Fatal("the fingerprint did not follow a new installed file")
	}
	RefreshTurnDependencies(root, "s1", 1)
	if TurnDependencies(root, "s1", 1) != DependencyFingerprint(root) {
		t.Fatal("the rebase did not record the installed tree")
	}
}
