package main

// Copies and cycles: a block of code that already exists in another file is reused, not copied (jscpd
// counts 5+ equal lines as a clone); an import that closes a loop between modules is refused
// (dependency-cruiser no-circular, eslint import/no-cycle).

import (
	"fmt"
	"hash/fnv"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// significantLines normalizes a file for clone detection: trimmed, spaces collapsed, without blank lines,
// lone braces, imports and comments. Each kept line remembers its line number.
func significantLines(code, ext string) ([]string, []int) {
	var lines []string
	var nums []int
	for i, l := range strings.Split(code, "\n") {
		t := strings.Join(strings.Fields(l), " ")
		if len(t) < 6 || strings.HasPrefix(t, "//") || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "/*") ||
			strings.HasPrefix(t, "import ") || strings.HasPrefix(t, "from ") || strings.HasPrefix(t, "package ") || strings.Trim(t, "{}()[];,") == "" {
			continue
		}
		lines = append(lines, t)
		nums = append(nums, i+1)
	}
	return lines, nums
}

func windowHash(lines []string) uint64 {
	h := fnv.New64a()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return h.Sum64()
}

type cloneIndex struct {
	mu    sync.Mutex
	files map[string]cloneFile // rel → its windows
}

type cloneFile struct {
	mod     int64
	windows map[uint64]int // hash → line
	imports []string       // resolved relative imports (JS/TS), for cycles
}

var cloneIndexes sync.Map // work root → *cloneIndex

// projectIndex returns the clone and import index of the project, refreshing the files that changed.
func projectIndex(work string, n int) map[string]cloneFile {
	v, _ := cloneIndexes.LoadOrStore(work, &cloneIndex{files: map[string]cloneFile{}})
	ix := v.(*cloneIndex)
	ix.mu.Lock()
	defer ix.mu.Unlock()
	seen := map[string]bool{}
	walkProjectFiles(work, func(rel, abs string, size int64) {
		ext := strings.ToLower(filepath.Ext(rel))
		if !codeExts[ext] || size > 300<<10 {
			return
		}
		seen[rel] = true
		info, err := os.Stat(abs)
		if err != nil {
			return
		}
		if f, ok := ix.files[rel]; ok && f.mod == info.ModTime().UnixNano() {
			return
		}
		b, err := os.ReadFile(abs)
		if err != nil {
			return
		}
		ix.files[rel] = cloneFile{mod: info.ModTime().UnixNano(), windows: windowsOf(string(b), ext, n), imports: relImports(rel, string(b), ext, work)}
	})
	for rel := range ix.files {
		if !seen[rel] {
			delete(ix.files, rel)
		}
	}
	out := make(map[string]cloneFile, len(ix.files))
	for k, f := range ix.files {
		out[k] = f
	}
	return out
}

func windowsOf(code, ext string, n int) map[uint64]int {
	lines, nums := significantLines(code, ext)
	out := map[uint64]int{}
	for i := 0; i+n <= len(lines); i++ {
		w := lines[i : i+n]
		if len(strings.Join(w, "")) < 120 { // short, generic lines are not a meaningful copy
			continue
		}
		if _, ok := out[windowHash(w)]; !ok {
			out[windowHash(w)] = nums[i]
		}
	}
	return out
}

// archDuplicates: the code a write adds may not be a copy of code in another file.
func archDuplicates(r *ArchRules, work, rel, ext, before, after string) string {
	if !r.on("duplicates") || !codeExts[ext] || ext == ".css" || r.DupLines < 3 {
		return ""
	}
	added := windowsOf(after, ext, r.DupLines)
	for h := range windowsOf(before, ext, r.DupLines) {
		delete(added, h)
	}
	if len(added) == 0 {
		return ""
	}
	for orel, f := range projectIndex(work, r.DupLines) {
		if orel == rel {
			continue
		}
		for h, line := range added {
			if oline, ok := f.windows[h]; ok {
				return fmt.Sprintf("o trecho a partir da linha %d repete %d+ linhas de %s:%d. Reaproveite: extraia uma função compartilhada (ou importe a que já existe) em vez de copiar.", line, r.DupLines, orel, oline)
			}
		}
	}
	return ""
}

var reComments = regexp.MustCompile(`(?s)/\*.*?\*/|(?m)//[^\n]*`)

var reRelImport = regexp.MustCompile(`(?:\bimport\s+(?:[^'"]*?\s+from\s+)?|\brequire\(\s*|\bimport\(\s*|\bexport\s+[^'"]*?\s+from\s+)['"](\.{1,2}/[^'"]+)['"]`)

// relImports resolves a JS/TS file's relative imports to project paths.
func relImports(rel, code, ext, work string) []string {
	if !jsLikeExts[ext] && ext != ".vue" && ext != ".svelte" {
		return nil
	}
	var out []string
	dir := path.Dir(rel)
	for _, m := range reRelImport.FindAllStringSubmatch(stripComments(code), -1) {
		target := path.Clean(path.Join(dir, m[1]))
		if t := resolveModule(work, target); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func stripComments(code string) string {
	return reComments.ReplaceAllString(code, "")
}

// resolveModule finds the file an import specifier points to (extension and index files as bundlers do).
func resolveModule(work, target string) string {
	cands := []string{target}
	for _, e := range []string{".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".vue", ".svelte"} {
		cands = append(cands, target+e, target+"/index"+e)
	}
	for _, c := range cands {
		if fi, err := os.Stat(filepath.Join(work, filepath.FromSlash(c))); err == nil && !fi.IsDir() {
			return c
		}
	}
	return ""
}

// archCycles: a new import that makes a module depend, through others, on itself.
func archCycles(r *ArchRules, work, rel, ext, before, after string) string {
	if !r.on("cycles") {
		return ""
	}
	newEdges := relImports(rel, after, ext, work)
	if len(newEdges) == 0 {
		return ""
	}
	had := map[string]bool{}
	for _, e := range relImports(rel, before, ext, work) {
		had[e] = true
	}
	graph := map[string][]string{}
	for f, cf := range projectIndex(work, max(3, r.DupLines)) {
		graph[f] = cf.imports
	}
	graph[rel] = newEdges
	deadline := time.Now().Add(2 * time.Second)
	for _, start := range newEdges {
		if had[start] {
			continue
		}
		// a path from the imported module back to this file closes a loop
		prev := map[string]string{start: ""}
		queue := []string{start}
		for len(queue) > 0 && time.Now().Before(deadline) {
			cur := queue[0]
			queue = queue[1:]
			if cur == rel {
				loop := []string{rel}
				for at := prev[rel]; at != ""; at = prev[at] {
					loop = append([]string{at}, loop...)
				}
				loop = append([]string{rel}, loop...)
				return "importar " + start + " cria um ciclo: " + strings.Join(loop, " → ") + ". Módulos dependem numa direção só: mova o que os dois usam para um módulo de baixo (compartilhado) que nenhum deles importa de volta."
			}
			for _, nx := range graph[cur] {
				if _, ok := prev[nx]; !ok {
					prev[nx] = cur
					queue = append(queue, nx)
				}
			}
		}
	}
	return ""
}
