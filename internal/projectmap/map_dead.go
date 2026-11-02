package projectmap

// Unreferenced exports: a symbol a file offers and no other mapped file mentions, and that the file
// itself does not use. Conservative on purpose: any mention counts, comments and prose do not.

import (
	"path"
	"strings"

	"infuse/internal/lang"
)

// DeadExport is one export with no reference. File is set when the whole file is unreachable.
type DeadExport struct {
	File   string `json:"file"`
	Symbol string `json:"symbol"`
	Whole  bool   `json:"wholeFile,omitempty"`
}

// tokenCounts counts the identifiers of a text, comment lines left out.
func tokenCounts(text string) map[string]int {
	out := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "//") || strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "/*") {
			continue
		}
		start := -1
		for i := 0; i <= len(line); i++ {
			isWord := i < len(line) && (line[i] == '_' || line[i] == '$' || line[i] >= '0' && line[i] <= '9' || line[i] >= 'a' && line[i] <= 'z' || line[i] >= 'A' && line[i] <= 'Z')
			if isWord && start < 0 {
				start = i
			} else if !isWord && start >= 0 {
				out[line[start:i]]++
				start = -1
			}
		}
	}
	return out
}

// entryNames are files the runtime reaches, not other code.
var entryNames = map[string]bool{"main": true, "index": true, "__init__": true, "__main__": true, "lib": true, "mod": true, "app": true}

// IsEntryFile reports whether a file is reached by the runtime rather than by an import.
func IsEntryFile(rel string) bool {
	base := path.Base(rel)
	stem := strings.TrimSuffix(base, path.Ext(base))
	return entryNames[stem] || strings.HasPrefix(rel, "cmd/") || strings.HasPrefix(rel, "bin/") || strings.Contains(rel, "/cmd/")
}

// goFrameworkNames are exports the Go tool calls by name.
func goFrameworkName(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Fuzz") || strings.HasPrefix(name, "Example")
}

// computeDead finds the exports nothing refers to, given the tokens of every file of a full build.
func computeDead(m *ProjectMap, c Contract) []DeadExport {
	users := map[string]string{} // token → one file using it, "" once two files do
	for _, f := range m.Files {
		if f.scan == nil || f.Lang == "md" {
			continue
		}
		for tok := range f.scan.tokens {
			if prev, seen := users[tok]; !seen {
				users[tok] = f.Path
			} else if prev != f.Path {
				users[tok] = ""
			}
		}
	}
	var out []DeadExport
	for _, f := range sortedFiles(m) {
		if f.scan == nil || !deadCandidate(f, c) {
			continue
		}
		var dead []string
		for _, sym := range f.Exports {
			if isDeadSymbol(f, sym, users) {
				dead = append(dead, sym)
			}
		}
		whole := len(dead) > 0 && len(dead) == len(f.Exports) && len(f.Dependents) == 0
		for _, sym := range dead {
			out = append(out, DeadExport{File: f.Path, Symbol: sym, Whole: whole})
		}
	}
	return out
}

func deadCandidate(f *FileInfo, c Contract) bool {
	return !nonCodeLangs[f.Lang] && !lang.IsTestPath(f.Path) && !IsEntryFile(f.Path) && !Within(f.Path, c.Public)
}

// isDeadSymbol: only this file mentions sym, once. A Go method may answer an interface nothing names
// here, so a Go name is judged only when it is declared at the top level.
func isDeadSymbol(f *FileInfo, sym string, users map[string]string) bool {
	if sym == "default" || f.scan.tokens[sym] > 1 || users[sym] != f.Path {
		return false
	}
	if f.Lang != "go" {
		return true
	}
	return !goFrameworkName(sym) && !strings.Contains(f.scan.text, ") "+sym+"(")
}
