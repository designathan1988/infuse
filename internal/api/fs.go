package api

// Folder listing behind the in-app folder browser. Hidden and system folders are skipped.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type folderEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type folderListing struct {
	Path   string        `json:"path"`
	Parent string        `json:"parent"`
	Dirs   []folderEntry `json:"dirs"`
	Roots  []folderEntry `json:"roots"`
}

func listFolders(p string) folderListing {
	out := folderListing{Dirs: []folderEntry{}, Roots: []folderEntry{}}
	home := core.HomeDir()
	out.Roots = append(out.Roots, folderEntry{Name: "Início", Path: home})
	for _, sub := range []string{"Documents", "Desktop", "Downloads", "Projects", "src", "dev", "code"} {
		if d := filepath.Join(home, sub); core.Exists(d) {
			out.Roots = append(out.Roots, folderEntry{Name: sub, Path: d})
		}
	}
	if runtime.GOOS == "windows" {
		for c := 'C'; c <= 'Z'; c++ {
			d := string(c) + `:\`
			if _, err := os.Stat(d); err == nil {
				out.Roots = append(out.Roots, folderEntry{Name: d, Path: d})
			}
		}
	} else {
		out.Roots = append(out.Roots, folderEntry{Name: "/", Path: "/"})
	}
	if p == "" {
		return out
	}
	p = filepath.Clean(p)
	out.Path = p
	if parent := filepath.Dir(p); parent != p {
		out.Parent = parent
	}
	entries, _ := os.ReadDir(p)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "$") || e.Name() == "node_modules" {
			continue
		}
		out.Dirs = append(out.Dirs, folderEntry{Name: e.Name(), Path: filepath.Join(p, e.Name())})
	}
	sort.Slice(out.Dirs, func(i, j int) bool { return strings.ToLower(out.Dirs[i].Name) < strings.ToLower(out.Dirs[j].Name) })
	return out
}
