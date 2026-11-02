package agent

import (
	"path/filepath"
	"strings"
)

// grepTypes maps Grep's type filter to file extensions. Names are ripgrep's where they exist.
var grepTypes = map[string][]string{
	"c":      {".c", ".h"},
	"cpp":    {".cc", ".cpp", ".cxx", ".hpp", ".hh", ".hxx", ".h"},
	"cs":     {".cs"},
	"css":    {".css", ".scss", ".sass", ".less"},
	"go":     {".go"},
	"html":   {".html", ".htm"},
	"java":   {".java"},
	"js":     {".js", ".mjs", ".cjs", ".jsx"},
	"json":   {".json"},
	"kotlin": {".kt", ".kts"},
	"md":     {".md", ".mdx", ".markdown"},
	"php":    {".php"},
	"py":     {".py", ".pyi"},
	"rb":     {".rb"},
	"rust":   {".rs"},
	"sh":     {".sh", ".bash", ".zsh"},
	"sql":    {".sql"},
	"swift":  {".swift"},
	"toml":   {".toml"},
	"ts":     {".ts", ".tsx", ".mts", ".cts"},
	"vue":    {".vue"},
	"xml":    {".xml"},
	"yaml":   {".yaml", ".yml"},
}

// grepTypeMatches reports whether a file belongs to the type; an empty type takes every file.
func grepTypeMatches(kind, rel string) bool {
	if kind == "" {
		return true
	}
	ext := strings.ToLower(filepath.Ext(rel))
	for _, e := range grepTypes[strings.ToLower(kind)] {
		if e == ext {
			return true
		}
	}
	return false
}

// grepTypeKnown reports whether the type is in the table.
func grepTypeKnown(kind string) bool {
	_, ok := grepTypes[strings.ToLower(kind)]
	return ok
}
