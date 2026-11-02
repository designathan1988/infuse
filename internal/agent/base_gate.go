package agent

// The architecture base is the user's structure: code written outside it, or across layers it does
// not allow, is refused before disk.

import (
	"infuse/internal/base"
	"path/filepath"
	"strings"
)

func (s *Session) baseViolation(rel string, isCode bool, content string) Msg {
	if name := base.CheckWrite(s.Root, rel, isCode); name != "" {
		return M("protocol.out_of_base", "file", rel, "base", name)
	}
	if !isCode {
		return Msg{}
	}
	if from, target := base.CheckLayers(s.Root, rel, content); from != "" {
		return M("protocol.layer", "file", rel, "rule", from, "target", target)
	}
	return Msg{}
}

// fileLineLimit is the size standard: the base's when the project chose one, else the user's setting
// (0 means 400, a negative value means no limit).
func (s *Session) fileLineLimit() int {
	if b := base.Load(s.Root); b != nil && b.MaxFileLines > 0 {
		return b.MaxFileLines
	}
	if s.settings.MaxFileLines == 0 {
		return 400
	}
	return s.settings.MaxFileLines
}

// baseGrounds is the base's whole contract as the prompt states it, including what the project actually
// has: declared folders that are missing and code folders that sit outside the base (intended vs. actual).
func baseGrounds(root string) string {
	b := base.Load(root)
	if b == nil {
		return ""
	}
	s := strings.TrimRight(b.Contract(), "\n")
	m := LoadMap(root)
	if m == nil {
		return s
	}
	var missing, extra []string
	for _, f := range b.Folders {
		if d, ok := m.Dirs[filepath.ToSlash(f)]; !ok || d.Intended { // Intended: declared, no code there yet
			missing = append(missing, f)
		}
	}
	for d, info := range m.Dirs {
		if d == "." || strings.Contains(d, "/") || strings.HasPrefix(d, ".") || b.Declares(d) {
			continue
		}
		for _, name := range info.Files {
			if codeExts[strings.ToLower(filepath.Ext(name))] {
				extra = append(extra, d)
				break
			}
		}
	}
	if len(missing) > 0 {
		s += "; declared folders not present yet: " + strings.Join(firstN(missing, 8), ", ")
	}
	if len(extra) > 0 {
		s += "; code folders outside the base: " + strings.Join(firstN(extra, 8), ", ")
	}
	return s
}
