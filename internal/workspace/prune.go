package workspace

// The blob store keeps the content of every file a turn snapshot saw. Without pruning it grows with
// every turn; this keeps the latest manifests of each session and removes content no kept manifest
// refers to.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// keptManifests is how many turns of a session remain rewindable through the blob store.
const keptManifests = 50

// PruneTree removes old turn manifests and the blobs they alone referenced. It is best-effort: a file
// it cannot read or remove is left in place.
func PruneTree(root string) {
	sessions, err := os.ReadDir(treeDir(root))
	if err != nil {
		return
	}
	referenced := map[string]bool{}
	for _, s := range sessions {
		if !s.IsDir() || s.Name() == "blobs" {
			continue
		}
		dir := filepath.Join(treeDir(root), s.Name())
		keep, drop := manifestsToKeep(dir)
		for _, name := range drop {
			_ = os.Remove(filepath.Join(dir, name))
		}
		for _, name := range keep {
			var m treeManifest
			if core.ReadJSON(filepath.Join(dir, name), &m) != nil {
				continue
			}
			for _, h := range m.Files {
				referenced[h] = true
			}
			for _, h := range m.Protected {
				referenced[h] = true
			}
		}
	}
	pruneBlobs(root, referenced)
}

// manifestsToKeep splits a session's turn files into the newest keptManifests and the older ones.
func manifestsToKeep(dir string) (keep, drop []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			if _, err := strconv.Atoi(strings.TrimSuffix(e.Name(), ".json")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	sort.Slice(names, func(i, j int) bool {
		a, _ := strconv.Atoi(strings.TrimSuffix(names[i], ".json"))
		b, _ := strconv.Atoi(strings.TrimSuffix(names[j], ".json"))
		return a < b
	})
	if len(names) <= keptManifests {
		return names, nil
	}
	return names[len(names)-keptManifests:], names[:len(names)-keptManifests]
}

func pruneBlobs(root string, referenced map[string]bool) {
	base := filepath.Join(treeDir(root), "blobs")
	prefixes, err := os.ReadDir(base)
	if err != nil {
		return
	}
	for _, p := range prefixes {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(base, p.Name())
		files, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !referenced[f.Name()] {
				_ = os.Remove(filepath.Join(dir, f.Name()))
			}
		}
		if rest, err := os.ReadDir(dir); err == nil && len(rest) == 0 {
			_ = os.Remove(dir)
		}
	}
}
