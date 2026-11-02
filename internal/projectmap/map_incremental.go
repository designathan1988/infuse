package projectmap

// Incremental update: the filter path touches one file and pays for one file. Features, drift and
// hotspots come from the map itself and are recomputed; the levels that need every file's text (data
// owners, examples, unreferenced exports) stay as they were until the next full build, which the
// watcher runs when writes stop.

import (
	"infuse/internal/core"
	"infuse/internal/fsx"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// updateLocks gives each project one writer of its map inside a process.
var updateLocks sync.Map // root → *sync.Mutex

func updateLock(root string) *sync.Mutex {
	v, _ := updateLocks.LoadOrStore(root, &sync.Mutex{})
	return v.(*sync.Mutex)
}

// UpdateFiles applies some changed files (project-relative) to the stored map and rewrites what they
// reach. Nil when no map exists yet: a full build owns the first map. It works on a private copy,
// because readers hold the shared one without a lock.
func UpdateFiles(root string, rels []string) *ProjectMap {
	mu := updateLock(root)
	mu.Lock()
	defer mu.Unlock()
	m := LoadMap(root)
	if m == nil || len(rels) == 0 {
		return m
	}
	applyUpdates(root, m, rels)
	return m
}

// RefreshDir brings one folder of the map up to date, as it is on disk now, and returns the map. With
// no stored map it answers from that folder alone, and nothing is stored.
func RefreshDir(root, rel string) *ProjectMap {
	mu := updateLock(root)
	mu.Lock()
	defer mu.Unlock()
	m := LoadMap(root)
	if m == nil {
		m = &ProjectMap{Root: root, Files: map[string]*FileInfo{}, Dirs: map[string]*DirInfo{}, partial: true}
	}
	rels, subdirsChanged := staleInDir(root, rel, m)
	if len(rels) > 0 || subdirsChanged {
		applyUpdates(root, m, rels)
	}
	return m
}

// staleInDir compares a folder with the map: the files to read again, and whether its subfolders moved.
func staleInDir(root, rel string, m *ProjectMap) ([]string, bool) {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, false
	}
	ensureDir(m, rel)
	d := m.Dirs[rel]
	present := map[string]bool{}
	var subdirs, rels []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if !fsx.SkipDir(name, fsx.Project) {
				subdirs = append(subdirs, name)
			}
			continue
		}
		present[name] = true
		if childChanged(m, join(rel, name), e) {
			rels = append(rels, join(rel, name))
		}
	}
	for _, name := range d.Files {
		if !present[name] {
			rels = append(rels, join(rel, name))
		}
	}
	return rels, syncSubdirs(m, d, subdirs)
}

func childChanged(m *ProjectMap, rel string, e os.DirEntry) bool {
	if _, ok := langOf(rel); !ok {
		return false
	}
	st, err := e.Info()
	if err != nil {
		return false
	}
	f := m.Files[rel]
	return f == nil || f.Bytes != st.Size() || st.ModTime().UnixMilli() >= m.GeneratedAt
}

// syncSubdirs makes a folder's subfolders match what is on disk; a removed one leaves with its tree.
func syncSubdirs(m *ProjectMap, d *DirInfo, now []string) bool {
	sort.Strings(now)
	had := map[string]bool{}
	for _, s := range d.Subdirs {
		had[s] = true
	}
	changed := len(now) != len(d.Subdirs)
	for _, s := range now {
		if !had[s] {
			changed = true
			m.Dirs[join(d.Path, s)] = &DirInfo{Path: join(d.Path, s), Files: []string{}, Subdirs: []string{}}
		}
		delete(had, s)
	}
	for s := range had {
		removeTree(m, join(d.Path, s))
	}
	d.Subdirs = now
	return changed
}

func removeTree(m *ProjectMap, rel string) {
	d := m.Dirs[rel]
	if d == nil {
		return
	}
	for _, s := range d.Subdirs {
		removeTree(m, join(rel, s))
	}
	for _, name := range d.Files {
		dropFile(m, join(rel, name))
	}
	adjustLines(m, rel, -d.Lines)
	delete(m.Dirs, rel)
}

func join(dir, name string) string {
	if dir == "" || dir == "." {
		return name
	}
	return dir + "/" + name
}

// applyUpdates applies the changes in memory and stores what they reach.
func applyUpdates(root string, m *ProjectMap, rels []string) {
	r := newResolver(root, m)
	touched := map[string]bool{}
	for _, rel := range rels {
		updateOne(r, rel, touched)
	}
	c := LoadContract(root)
	applyContract(m, c)
	computeHotspots(m, c.MaxFileLines)
	m.Drift = computeDrift(m, c)
	dropGone(m)
	m.Features, m.Routes = buildFeatures(m)
	m.GeneratedAt = time.Now().UnixMilli()
	if m.partial {
		return
	}
	writeMapJSON(root, m)
	writeIfChanged(mapDocFile(root), mapDocument(m))
	writeDirDocs(root, m, keys(withAncestors(touched)))
	remember(root, m)
}

func withAncestors(dirs map[string]bool) map[string]bool {
	out := map[string]bool{}
	for d := range dirs {
		for {
			out[d] = true
			if d == "." {
				break
			}
			d = path.Dir(d)
		}
	}
	return out
}

// updateOne reads one file again and moves its edges in the import graph.
func updateOne(r *resolver, rel string, touched map[string]bool) {
	m := r.m
	dir := path.Dir(rel)
	touched[dir] = true
	old := m.Files[rel]
	next := readFileInfo(r.root, filepath.Join(r.root, filepath.FromSlash(rel)), false)
	if old != nil {
		unlinkImports(m, old)
	}
	if next == nil {
		if old != nil {
			adjustLines(m, dir, -old.Lines)
			dropFile(m, rel)
		}
		return
	}
	if old != nil {
		next.Dependents = old.Dependents
		adjustLines(m, dir, next.Lines-old.Lines)
	} else {
		m.Files[rel] = next
		r.index(rel, next)
		sort.Strings(r.goPkgs[dir])
		next.Dependents = packageDependents(r, next)
		ensureDir(m, dir)
		addDirFile(m.Dirs[dir], path.Base(rel))
		adjustLines(m, dir, next.Lines)
	}
	m.Files[rel] = next
	r.resolveFile(next)
	for _, imp := range next.Imports {
		if t := m.Files[imp]; t != nil && t != next && !core.Contains(t.Dependents, rel) {
			t.Dependents = append(t.Dependents, rel)
			sort.Strings(t.Dependents)
		}
	}
}

// packageDependents: a new Go file joins a package its importers already depend on.
func packageDependents(r *resolver, f *FileInfo) []string {
	out := []string{}
	if f.Lang != "go" {
		return out
	}
	for _, sibling := range r.goPkgs[path.Dir(f.Path)] {
		if t := r.m.Files[sibling]; t != nil && sibling != f.Path {
			out = dedupe(out, t.Dependents...)
		}
	}
	return out
}

func unlinkImports(m *ProjectMap, f *FileInfo) {
	for _, imp := range f.Imports {
		if t := m.Files[imp]; t != nil {
			t.Dependents = without(t.Dependents, f.Path)
		}
	}
}

func without(list []string, s string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != s {
			out = append(out, v)
		}
	}
	return out
}

// dropFile takes a file out of the map and out of its folder's list.
func dropFile(m *ProjectMap, rel string) {
	if f := m.Files[rel]; f != nil {
		unlinkImports(m, f)
	}
	delete(m.Files, rel)
	if d := m.Dirs[path.Dir(rel)]; d != nil {
		d.Files = without(d.Files, path.Base(rel))
	}
}

func addDirFile(d *DirInfo, name string) {
	if !core.Contains(d.Files, name) {
		d.Files = append(d.Files, name)
		sort.Strings(d.Files)
	}
}

func ensureDir(m *ProjectMap, rel string) {
	if _, ok := m.Dirs[rel]; !ok {
		m.Dirs[rel] = &DirInfo{Path: rel, Files: []string{}, Subdirs: []string{}}
	}
	linkSubdir(m, rel)
}

// adjustLines moves a folder's line count and every ancestor's.
func adjustLines(m *ProjectMap, rel string, delta int) {
	for {
		if d := m.Dirs[rel]; d != nil {
			d.Lines += delta
		}
		if rel == "." {
			return
		}
		rel = path.Dir(rel)
	}
}

// dropGone removes from the slower levels what they say about files that left.
func dropGone(m *ProjectMap) {
	examples := m.Examples[:0]
	for _, e := range m.Examples {
		if m.Files[e.File] != nil {
			examples = append(examples, e)
		}
	}
	m.Examples = examples
	dead := m.Dead[:0]
	for _, e := range m.Dead {
		if m.Files[e.File] != nil {
			dead = append(dead, e)
		}
	}
	m.Dead = dead
	rows := m.Data[:0]
	for _, e := range m.Data {
		if m.Files[e.File] != nil {
			rows = append(rows, e)
		}
	}
	m.Data = rows
}
