package oracle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The snapshot's dependency folder is a copy: what a check does to it never reaches the project.
func TestDependenciesAreCopiedNotLinked(t *testing.T) {
	testHome(t)
	requireContainment(t)
	root := t.TempDir()
	dep := filepath.Join(root, "node_modules", "pkg", "dist", "index.js")
	mustWrite(t, dep, "real\n")
	mustWrite(t, filepath.Join(root, "node_modules", ".cache", "big"), "cache\n")
	writeSpec(t, root, commandSpec(`grep -q real node_modules/pkg/dist/index.js && test ! -e node_modules/.cache && echo patched > node_modules/pkg/dist/index.js`))
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("%+v", v.Checks)
	}
	if b, _ := os.ReadFile(dep); string(b) != "real\n" {
		t.Fatalf("a check reached the live dependency folder: %q", b)
	}
}

// Patching an installed tool without touching what declares it fails the run, whatever the patched tool
// would have answered; reinstalling restores the hash, and a declared change is accepted and stated.
func TestPatchedDependencyIsRefused(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	jest := filepath.Join(root, "node_modules", ".bin", "jest")
	lock := filepath.Join(root, "package-lock.json")
	mustWrite(t, jest, "#!/bin/sh\nexit 1\n")
	mustWrite(t, lock, `{"v":1}`)
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"package-lock.json"}]}`)
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("baseline run: %+v", v)
	}
	mustWrite(t, jest, "#!/bin/sh\nexit 0\n")
	if v := run(t, root); v.Verdict != "erro" || !strings.Contains(v.Summary, "node_modules") {
		t.Fatalf("a patched dependency must not reach a verdict: %+v", v)
	}
	if fp := DependencyFingerprint(root); fp == "" {
		t.Fatal("no fingerprint")
	}
	mustWrite(t, jest, "#!/bin/sh\nexit 1\n")
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("restoring the folder must clear the refusal: %+v", v)
	}
	mustWrite(t, jest, "#!/bin/sh\nexit 2\n")
	mustWrite(t, lock, `{"v":2}`)
	if v := run(t, root); v.Verdict != "aprovado" || len(v.Warnings) == 0 {
		t.Fatalf("a declared update is accepted and stated: %+v", v)
	}
}

// The baseline taken at the start of a turn catches a patch made before the first run of the executor.
func TestBaselineFromTheTurnStartCatchesAFirstRunPatch(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	jest := filepath.Join(root, "node_modules", ".bin", "jest")
	mustWrite(t, jest, "#!/bin/sh\nexit 1\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"node_modules/.bin/jest"}]}`)
	RecordDependencies(root, root)
	mustWrite(t, jest, "#!/bin/sh\nexit 0\n")
	if v := run(t, root); v.Verdict != "erro" {
		t.Fatalf("%+v", v)
	}
}

// Saving the criteria is the user accepting the folders as they are.
func TestSavingTheCriteriaAcceptsTheDependencies(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	jest := filepath.Join(root, "node_modules", ".bin", "jest")
	mustWrite(t, jest, "a\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"node_modules/.bin/jest"}]}`)
	run(t, root)
	mustWrite(t, jest, "b\n")
	writeSpec(t, root, `{"checks":[{"id":"a","kind":"file","path":"node_modules/.bin/jest"}]}`)
	if v := run(t, root); v.Verdict != "aprovado" {
		t.Fatalf("%+v", v)
	}
}

// The cached-verdict fingerprint moves with the content of a dependency, even at the same size.
func TestDependencyFingerprintFollowsContent(t *testing.T) {
	testHome(t)
	root := t.TempDir()
	dep := filepath.Join(root, ".venv", "lib", "x.py")
	mustWrite(t, dep, "a = 1\n")
	before := DependencyFingerprint(root)
	mustWrite(t, dep, "a = 2\n")
	if DependencyFingerprint(root) == before {
		t.Fatal("a same-size patch left the fingerprint unchanged")
	}
}
