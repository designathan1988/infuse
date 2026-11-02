package projectmap

// Feature index: where each feature of the project lives. Built from feature folders, HTTP route
// literals and the name clusters (map_clusters.go) — deterministic, sorted and bounded.

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	maxFeatures      = 60
	maxFeaturePaths  = 8
	maxFeatureRoutes = 6
	maxRoutes        = 400
)

// Feature is one feature and the places its code is split across. Paths is the bounded list a reader
// sees; Files, Dirs and Unlinked come from the name cluster, whole: Unlinked lists folders of the
// feature no import ties to the rest.
type Feature struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`         // folder | prefix | route
	Stem      string   `json:"stem,omitempty"` // the word its file names share
	Paths     []string `json:"paths"`
	PathCount int      `json:"pathCount"`
	FileCount int      `json:"fileCount"`
	Lines     int      `json:"lines"`
	Routes    []string `json:"routes,omitempty"` // "GET /users"
	Files     []string `json:"files,omitempty"`
	Dirs      []string `json:"dirs,omitempty"`
	Unlinked  []string `json:"unlinked,omitempty"`
}

var featureRoots = map[string]bool{
	"features": true, "modules": true, "pages": true, "commands": true, "routes": true,
	"handlers": true, "views": true, "controllers": true, "screens": true, "endpoints": true,
	"domains": true, "usecases": true, "use-cases": true, "services": true,
}

var genericFeatureNames = map[string]bool{
	"index": true, "main": true, "app": true, "init": true, "config": true, "configs": true,
	"types": true, "type": true, "constants": true, "consts": true, "utils": true, "util": true,
	"helpers": true, "helper": true, "common": true, "shared": true, "lib": true, "core": true,
	"test": true, "tests": true, "spec": true, "specs": true, "setup": true, "server": true,
	"client": true, "models": true, "components": true, "store": true, "api": true, "data": true,
	"styles": true, "assets": true, "layout": true, "error": true, "errors": true,
}

var genericRouteSegments = map[string]bool{
	"api": true, "static": true, "assets": true, "public": true, "health": true, "healthz": true,
	"ready": true, "metrics": true, "index": true, "app": true, "_next": true, "favicon.ico": true,
}

var reVersionSegment = regexp.MustCompile(`^v\d+$`)

type featureAgg struct {
	name       string
	source     string
	paths      map[string]bool
	files      int
	lines      int
	routes     []string
	seenRoutes map[string]bool
	cluster    Feature // the name cluster folded into this feature
}

func newAgg(name, source string) *featureAgg {
	return &featureAgg{name: name, source: source, paths: map[string]bool{}, seenRoutes: map[string]bool{}}
}

func (a *featureAgg) addPath(p string, lines int) {
	a.paths[p] = true
	a.files++
	a.lines += lines
}

func aggFor(aggs map[string]*featureAgg, name, source string) *featureAgg {
	key := strings.ToLower(name)
	a := aggs[key]
	if a == nil {
		a = newAgg(name, source)
		aggs[key] = a
	}
	return a
}

func sortedMapFiles(m *ProjectMap) []string {
	files := make([]string, 0, len(m.Files))
	for rel := range m.Files {
		files = append(files, rel)
	}
	sort.Strings(files)
	return files
}

// folderFeature locates the feature a file belongs to through a feature root
// (features/, pages/, commands/, handlers/, ...). A file directly inside a root is a feature itself.
func folderFeature(rel string) (string, string, bool) {
	segs := strings.Split(rel, "/")
	for i := 0; i+1 < len(segs); i++ {
		if !featureRoots[strings.ToLower(segs[i])] {
			continue
		}
		if i+2 == len(segs) {
			name := strings.TrimSuffix(segs[i+1], path.Ext(segs[i+1]))
			if usableFeatureName(name) {
				return name, rel, true
			}
			return "", "", false
		}
		if usableFeatureName(segs[i+1]) {
			return segs[i+1], strings.Join(segs[:i+2], "/"), true
		}
		return "", "", false
	}
	return "", "", false
}

func usableFeatureName(name string) bool {
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" || genericFeatureNames[low] || strings.HasPrefix(low, "_") || strings.HasPrefix(low, ".") {
		return false
	}
	return !(strings.HasPrefix(low, "[") && strings.HasSuffix(low, "]")) // Next.js dynamic segment
}

func addFolderFeatures(m *ProjectMap, aggs map[string]*featureAgg, covered map[string]bool) {
	for _, rel := range sortedMapFiles(m) {
		name, where, ok := folderFeature(rel)
		if !ok {
			continue
		}
		aggFor(aggs, name, "folder").addPath(where, m.Files[rel].Lines)
		covered[rel] = true
	}
}

// addClusterFeatures folds in the name clusters: files named billing_service.py and billing_repo.py
// form the feature "billing", or join the folder feature of that name.
func addClusterFeatures(m *ProjectMap, aggs map[string]*featureAgg, covered map[string]bool) {
	for _, c := range computeClusters(m) {
		name := c.Stem
		if w := featureWord(c.Files[0]); featureStem(c.Files[0]) == c.Stem {
			name = w
		}
		a := aggs[strings.ToLower(c.Stem)]
		if a == nil {
			a = aggFor(aggs, name, "prefix")
		}
		a.cluster = c
		for _, rel := range c.Files {
			if !covered[rel] && !isTestFile(path.Base(rel)) {
				a.addPath(rel, m.Files[rel].Lines)
			}
		}
	}
}

func isTestFile(base string) bool {
	low := strings.ToLower(base)
	return strings.Contains(low, ".test.") || strings.Contains(low, ".spec.") ||
		strings.Contains(low, "_test.") || strings.HasPrefix(low, "test_")
}

// addRouteFeatures folds routes into the feature named by their first static path segment.
func addRouteFeatures(m *ProjectMap, aggs map[string]*featureAgg, routes []Route) {
	for _, r := range routes {
		name := routeFeatureName(r.Path)
		if name == "" {
			continue
		}
		a := aggFor(aggs, name, "route")
		if a.source == "route" && !a.paths[r.File] {
			a.paths[r.File] = true
			a.files++
			if f := m.Files[r.File]; f != nil {
				a.lines += f.Lines
			}
		}
		label := r.Method + " " + r.Path
		if r.Method == "*" {
			label = "ANY " + r.Path
		}
		if len(a.routes) < maxFeatureRoutes && !a.seenRoutes[label] {
			a.seenRoutes[label] = true
			a.routes = append(a.routes, label)
		}
	}
}

func routeFeatureName(p string) string {
	for _, seg := range strings.Split(p, "/") {
		low := strings.ToLower(seg)
		if seg == "" || low == "*" || genericRouteSegments[low] || reVersionSegment.MatchString(low) {
			continue
		}
		if strings.HasPrefix(seg, "{") || strings.HasPrefix(seg, ":") || strings.HasPrefix(seg, "<") {
			continue
		}
		if strings.ContainsAny(seg, "[]()^$?+|\\*") || !usableFeatureName(seg) {
			continue
		}
		return low
	}
	return ""
}

func finalizeFeatures(aggs map[string]*featureAgg) []Feature {
	out := make([]Feature, 0, len(aggs))
	for _, a := range aggs {
		if a.source == "prefix" && a.files < 2 && len(a.cluster.Files) < 2 {
			continue
		}
		paths := make([]string, 0, len(a.paths))
		for p := range a.paths {
			paths = append(paths, p)
		}
		sort.Strings(paths)
		f := Feature{Name: a.name, Source: a.source, Stem: a.cluster.Stem, PathCount: len(paths), FileCount: a.files, Lines: a.lines, Routes: a.routes,
			Files: a.cluster.Files, Dirs: a.cluster.Dirs, Unlinked: a.cluster.Unlinked}
		if len(paths) > maxFeaturePaths {
			paths = paths[:maxFeaturePaths]
		}
		f.Paths = paths
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].FileCount != out[j].FileCount {
			return out[i].FileCount > out[j].FileCount
		}
		if out[i].Lines != out[j].Lines {
			return out[i].Lines > out[j].Lines
		}
		return out[i].Name < out[j].Name
	})
	return boundFeatures(out)
}

// boundFeatures keeps the largest features, and every feature split across unlinked folders: that is
// the fact the structure audit needs, whatever its size.
func boundFeatures(list []Feature) []Feature {
	if len(list) <= maxFeatures {
		return list
	}
	out := list[:maxFeatures:maxFeatures]
	for _, f := range list[maxFeatures:] {
		if len(f.Unlinked) > 0 {
			out = append(out, f)
		}
	}
	return out
}

func collectRoutes(m *ProjectMap) []Route {
	out := make([]Route, 0, len(m.Routes))
	seen := map[string]bool{}
	for _, rel := range sortedMapFiles(m) {
		for _, r := range m.Files[rel].Routes {
			key := r.Method + " " + r.Path + " " + r.File
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		if out[i].Method != out[j].Method {
			return out[i].Method < out[j].Method
		}
		return out[i].File < out[j].File
	})
	if len(out) > maxRoutes {
		out = out[:maxRoutes]
	}
	return out
}

// buildFeatures fills the feature index and the route list after the file walk.
func buildFeatures(m *ProjectMap) ([]Feature, []Route) {
	aggs := map[string]*featureAgg{}
	covered := map[string]bool{}
	addFolderFeatures(m, aggs, covered)
	addClusterFeatures(m, aggs, covered)
	routes := collectRoutes(m)
	addRouteFeatures(m, aggs, routes)
	return finalizeFeatures(aggs), routes
}

// FeatureSummary renders the index for the agent prompt.
func (m *ProjectMap) FeatureSummary(max int) string {
	if m == nil || len(m.Features) == 0 {
		return ""
	}
	var b strings.Builder
	for i, f := range m.Features {
		if i >= max {
			fmt.Fprintf(&b, "- … %d more feature(s)\n", len(m.Features)-max)
			break
		}
		fmt.Fprintf(&b, "- %s (%d files): %s", f.Name, f.FileCount, strings.Join(f.Paths, ", "))
		if f.PathCount > len(f.Paths) {
			fmt.Fprintf(&b, " (+%d paths)", f.PathCount-len(f.Paths))
		}
		if len(f.Routes) > 0 {
			fmt.Fprintf(&b, "; %s", strings.Join(f.Routes, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// appendFeaturesMD writes the feature index and the HTTP surface into MAP.md.
func appendFeaturesMD(md *strings.Builder, m *ProjectMap) {
	if len(m.Features) > 0 {
		md.WriteString("\n## Features\n")
		for _, f := range m.Features {
			fmt.Fprintf(md, "- %s (%d files, %d lines): %s", f.Name, f.FileCount, f.Lines, strings.Join(f.Paths, ", "))
			if f.PathCount > len(f.Paths) {
				fmt.Fprintf(md, " (+%d more paths)", f.PathCount-len(f.Paths))
			}
			md.WriteString("\n")
		}
	}
	if len(m.Routes) > 0 {
		md.WriteString("\n## HTTP routes\n")
		for _, r := range m.Routes {
			fmt.Fprintf(md, "- %s %s → %s\n", r.Method, r.Path, r.File)
		}
	}
}
