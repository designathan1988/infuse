package projectmap

// Native per-folder context files (SPEC §3): CLAUDE.md, AGENTS.md and GEMINI.md are generated from the
// map on request, inside a marked block, so the agent reading a folder reads that folder's map without
// infuse being called. What the user wrote around the block is never touched. Off by default for an
// existing repository, on for a scaffold.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
)

// ContextFileNames are the files each agent reads natively, per folder.
var ContextFileNames = []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md"}

// BlockMap names the per-folder block; the launcher writes its own block in the same files.
const BlockMap = "map"

func blockBegin(name string) string {
	return "<!-- infuse:begin " + name + " (generated; edits inside this block are replaced) -->"
}
func blockEnd(name string) string { return "<!-- infuse:end " + name + " -->" }

func isContextFile(name string) bool {
	for _, n := range ContextFileNames {
		if strings.EqualFold(n, name) {
			return true
		}
	}
	return false
}

// isContextDir: the root and folders that hold mapped files of their own are worth a context file.
func isContextDir(m *ProjectMap, rel string) bool {
	d := m.Dirs[rel]
	if d == nil {
		return false
	}
	if rel == "." {
		return true
	}
	for _, name := range d.Files {
		if !isContextFile(name) {
			return true
		}
	}
	return d.Role != ""
}

type contextSettings struct {
	Enabled bool `json:"enabled"`
}

func contextSettingsFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "context.json")
}

// ContextFilesEnabled reports whether the project asked for generated context files.
func ContextFilesEnabled(root string) bool {
	var c contextSettings
	return core.ReadJSON(contextSettingsFile(root), &c) == nil && c.Enabled
}

// SetContextFiles turns the generated context files on (writing them from the map) or off (removing
// the generated blocks, and a file left empty by that).
func SetContextFiles(root string, on bool) error {
	if err := core.WriteJSON(contextSettingsFile(root), contextSettings{Enabled: on}); err != nil {
		return err
	}
	m := LoadOrBuildMap(root)
	for _, d := range sortedDirs(m) {
		if on && isContextDir(m, d) {
			writeContextFiles(root, d, describe(m, d, false)+"\n")
		} else if !on {
			removeContextFiles(root, d)
		}
	}
	return nil
}

func hasContextFiles(root, rel string) bool {
	return core.Exists(filepath.Join(root, filepath.FromSlash(rel), ContextFileNames[0]))
}

func writeContextFiles(root, rel, doc string) {
	dir := filepath.Join(root, filepath.FromSlash(rel))
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return
	}
	for _, name := range ContextFileNames {
		_ = UpsertBlock(filepath.Join(dir, name), BlockMap, doc)
	}
}

func removeContextFiles(root, rel string) {
	for _, name := range ContextFileNames {
		_ = RemoveBlock(filepath.Join(root, filepath.FromSlash(rel), name), BlockMap)
	}
}

// UpsertBlock writes a named block into a file, replacing that block when it exists and appending it
// after the user's content otherwise. It never changes anything outside the block.
func UpsertBlock(file, name, body string) error {
	b, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	block := blockBegin(name) + "\n" + strings.TrimRight(body, "\n") + "\n" + blockEnd(name)
	text := string(b)
	next := ""
	if start, end, ok := blockRange(text, name); ok {
		next = text[:start] + block + text[end:]
	} else if strings.TrimSpace(text) == "" {
		next = block + "\n"
	} else {
		next = strings.TrimRight(text, "\r\n") + "\n\n" + block + "\n"
	}
	if next == text {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(next), 0o644)
}

// RemoveBlock takes a named block out of a file and deletes a file left with nothing in it.
func RemoveBlock(file, name string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	text := string(b)
	start, end, ok := blockRange(text, name)
	if !ok {
		return nil
	}
	next := strings.TrimRight(text[:start], "\r\n") + text[end:]
	if strings.TrimSpace(next) == "" {
		return os.Remove(file)
	}
	return os.WriteFile(file, []byte(strings.TrimRight(next, "\r\n")+"\n"), 0o644)
}

// blockRange locates a named block, markers included. An unterminated block is not a block: nothing
// the user wrote after a lost end marker may be replaced.
func blockRange(text, name string) (int, int, bool) {
	start := strings.Index(text, "<!-- infuse:begin "+name+" ")
	if start < 0 {
		return 0, 0, false
	}
	endMark := blockEnd(name)
	n := strings.Index(text[start:], endMark)
	if n < 0 {
		return 0, 0, false
	}
	return start, start + n + len(endMark), true
}

// StripGeneratedBlocks removes every generated block from an instruction text: what infuse wrote
// there is the map, not the user's instructions, and must not become locks or required reading.
func StripGeneratedBlocks(text string) string {
	for {
		i := strings.Index(text, "<!-- infuse:begin ")
		if i < 0 {
			return text
		}
		rest := text[i+len("<!-- infuse:begin "):]
		sp := strings.IndexByte(rest, ' ')
		if sp < 0 {
			return text
		}
		start, end, ok := blockRange(text, rest[:sp])
		if !ok {
			return text
		}
		text = text[:start] + text[end:]
	}
}
