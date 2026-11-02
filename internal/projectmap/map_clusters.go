package projectmap

// Name clusters: files group by the stem their names share, and a file with no stem of its own joins
// the one cluster that is the only thing importing it. The feature index folds each cluster in.

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

// genericStems say nothing about a feature.
var genericStems = map[string]bool{
	"index": true, "main": true, "app": true, "test": true, "tests": true, "spec": true, "util": true, "utils": true,
	"helper": true, "helpers": true, "common": true, "base": true, "core": true, "type": true, "types": true,
	"mod": true, "lib": true, "init": true, "new": true, "get": true, "set": true, "data": true, "config": true,
	"model": true, "models": true, "service": true, "services": true, "shared": true, "misc": true, "api": true,
}

var (
	reCamelBreak = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	reNameSplit  = regexp.MustCompile(`[^A-Za-z0-9]+`)
)

// featureStem is the first meaningful word of a file's name.
func featureStem(rel string) string {
	word := featureWord(rel)
	if len(word) > 3 && strings.HasSuffix(word, "s") {
		return strings.TrimSuffix(word, "s")
	}
	return word
}

// featureWord is that word as the name spells it.
func featureWord(rel string) string {
	name := path.Base(rel)
	name = strings.TrimSuffix(name, path.Ext(name))
	for _, part := range reNameSplit.Split(reCamelBreak.ReplaceAllString(name, "${1}_${2}"), -1) {
		s := strings.ToLower(part)
		stem := strings.TrimSuffix(s, "s")
		if len(s) <= 3 {
			stem = s
		}
		if len(s) > 2 && !genericStems[stem] {
			return s
		}
	}
	return ""
}

// computeClusters clusters the code files.
func computeClusters(m *ProjectMap) []Feature {
	groups := map[string][]string{}
	stemOf := map[string]string{}
	for _, f := range sortedFiles(m) {
		if nonCodeLangs[f.Lang] {
			continue
		}
		if s := featureStem(f.Path); s != "" {
			groups[s] = append(groups[s], f.Path)
			stemOf[f.Path] = s
		}
	}
	for s, files := range groups {
		if len(files) < 2 {
			delete(groups, s)
			delete(stemOf, files[0])
		}
	}
	adoptExclusiveImports(m, groups, stemOf)
	out := make([]Feature, 0, len(groups))
	for s, files := range groups {
		sort.Strings(files)
		out = append(out, describeFeature(m, s, files))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// adoptExclusiveImports adds to a feature the file only that feature imports.
func adoptExclusiveImports(m *ProjectMap, groups map[string][]string, stemOf map[string]string) {
	for _, f := range sortedFiles(m) {
		if nonCodeLangs[f.Lang] || stemOf[f.Path] != "" || len(f.Dependents) == 0 {
			continue
		}
		owner := ""
		for _, d := range f.Dependents {
			s := stemOf[d]
			if s == "" || owner != "" && s != owner {
				owner = ""
				break
			}
			owner = s
		}
		if owner != "" {
			groups[owner] = append(groups[owner], f.Path)
		}
	}
}

// describeFeature finds the folders the feature lives in and which of them nothing connects: the
// largest group of folders tied by imports is the feature's body, the others stand alone.
func describeFeature(m *ProjectMap, name string, files []string) Feature {
	inFeature := map[string]bool{}
	dirs := map[string]bool{}
	parent := map[string]string{}
	for _, f := range files {
		inFeature[f] = true
		d := path.Dir(f)
		dirs[d] = true
		parent[d] = d
	}
	var find func(string) string
	find = func(x string) string {
		if parent[x] != x {
			parent[x] = find(parent[x])
		}
		return parent[x]
	}
	for _, f := range files {
		for _, imp := range m.Files[f].Imports {
			if inFeature[imp] {
				parent[find(path.Dir(f))] = find(path.Dir(imp))
			}
		}
	}
	size := map[string]int{}
	for d := range dirs {
		size[find(d)]++
	}
	dirList := keys(dirs)
	body, best := "", 0
	for _, d := range dirList {
		if r := find(d); size[r] > best {
			body, best = r, size[r]
		}
	}
	fe := Feature{Name: name, Stem: name, Files: files, Dirs: dirList}
	for _, d := range dirList {
		if find(d) != body {
			fe.Unlinked = append(fe.Unlinked, d)
		}
	}
	return fe
}

// FeatureOf returns the feature a file's name belongs to.
func FeatureOf(m *ProjectMap, rel string) (Feature, bool) {
	s := featureStem(rel)
	for _, f := range m.Features {
		if f.Stem == s {
			return f, true
		}
	}
	return Feature{}, false
}
