package oracle

// Dependencies are code the checks run but the turn audit does not see: patching node_modules/.bin/jest
// to exit 0 changes no project file. Each dependency folder's content hash is kept in the user's store,
// next to the criteria anchor. A folder that changed while the files that declare it did not is
// refused; the remedy is reinstalling from the lockfile, which restores the hash. A folder that changed
// together with its manifest or lockfile is accepted and stated with the verdict, and that manifest is
// a project file the audit does see. Saving the criteria accepts the folders as they are.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// dependencyDirs are the folders where a package manager installs code. vendor is not one of them: a
// project may own that folder, and it is copied like any other when git tracks it.
var dependencyDirs = map[string]bool{"node_modules": true, ".venv": true, "venv": true}

// dependencyCaches are rewritten by ordinary runs: neither copied nor hashed.
var dependencyCaches = map[string]bool{".cache": true, ".vite": true, ".vitest": true, "__pycache__": true}

// declarationFiles are what a dependency folder is installed from.
var declarationFiles = []string{"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock", "bun.lock", "bun.lockb", "pyproject.toml", "poetry.lock", "uv.lock", "Pipfile", "Pipfile.lock", "setup.py", "setup.cfg", "requirements*.txt"}

type depTree struct {
	h hash.Hash
	n int
}

func sortedKeys(m map[string]*depTree) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// dependencyOf returns the dependency folder a slash path lies in, the folder itself included.
func dependencyOf(rel string) string {
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		if dependencyDirs[s] {
			return strings.Join(segs[:i+1], "/")
		}
	}
	return ""
}

// dependencyTrees hashes the dependency folders of a work tree without copying them.
func dependencyTrees(work string) (map[string]string, error) {
	c := newCopier(work, "")
	c.depsOnly = true
	if err := c.run(); err != nil {
		return nil, err
	}
	return c.trees, nil
}

// DependencyFingerprint identifies the installed dependencies: it joins the key of a cached verdict, so
// a patched folder never replays an approval.
func DependencyFingerprint(work string) string { return fsx.DependencyFingerprint(work) }

// declarationHash hashes the files that declare the given dependency folders.
func declarationHash(work string, trees map[string]string) string {
	dirs := map[string]bool{}
	for t := range trees {
		dirs[path.Dir(t)] = true
	}
	var files []string
	for d := range dirs {
		for _, name := range declarationFiles {
			found, _ := filepath.Glob(filepath.Join(work, filepath.FromSlash(d), name))
			files = append(files, found...)
		}
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err == nil {
			fmt.Fprintf(h, "%s %d\n", filepath.Base(f), len(b))
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// depsRecord is the baseline kept in the user's store.
type depsRecord struct {
	Declared string            `json:"declared"`
	Trees    map[string]string `json:"trees"`
}

func depsFile(work string) string { return anchorFile(work) + ".deps" }

// checkDependencies compares the snapshot's folders with the baseline. refused is why the run must not
// be trusted; warnings are stated with the verdict. The first run records the baseline.
func checkDependencies(work string, trees map[string]string) (warnings []string, refused string) {
	declared := declarationHash(work, trees)
	var rec depsRecord
	if err := core.ReadJSON(depsFile(work), &rec); err != nil {
		saveDeps(work, depsRecord{Declared: declared, Trees: trees})
		return nil, ""
	}
	var changed, added []string
	for folder, h := range trees {
		old, known := rec.Trees[folder]
		switch {
		case !known:
			added = append(added, folder)
		case old != h:
			changed = append(changed, folder)
		}
	}
	sort.Strings(changed)
	if len(changed) > 0 && declared == rec.Declared {
		return nil, uiText("oracle.dependenciesChanged", "folders", strings.Join(changed, ", "))
	}
	if moved := append(changed, added...); len(moved) > 0 {
		sort.Strings(moved)
		warnings = append(warnings, uiText("oracle.dependenciesUpdated", "folders", strings.Join(moved, ", ")))
	}
	if rec.Trees == nil {
		rec.Trees = map[string]string{}
	}
	for folder, h := range trees {
		rec.Trees[folder] = h
	}
	rec.Declared = declared
	saveDeps(work, rec)
	return warnings, ""
}

func saveDeps(work string, rec depsRecord) {
	protectStore()
	_ = core.WriteJSON(depsFile(work), rec)
}

// RecordDependencies takes the baseline before an agent turn can touch the folders, once per project
// with criteria; without it the first baseline would be whatever the first run finds.
func RecordDependencies(root, work string) {
	if spec, err := LoadOracleSpec(root); err != nil || len(spec.Checks) == 0 || core.Exists(depsFile(work)) {
		return
	}
	if trees, err := dependencyTrees(work); err == nil {
		saveDeps(work, depsRecord{Declared: declarationHash(work, trees), Trees: trees})
	}
}
