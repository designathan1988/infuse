package agent

// The architecture base is the user's structure: code written outside it is refused before disk.

import (
	"infuse/internal/base"
	"path/filepath"
	"strings"
)

func (s *Session) baseViolation(rel string, isCode bool) Msg {
	if name := base.CheckWrite(s.Root, rel, isCode); name != "" {
		return M("protocol.out_of_base", "file", rel, "base", name)
	}
	return Msg{}
}

// baseGrounds is the base as the prompt states it, including what the project actually has: declared
// folders that are missing and code folders that sit outside the base (intended vs. actual).
func baseGrounds(root string) string {
	b := base.Load(root)
	if b == nil {
		return ""
	}
	s := b.Summary()
	m := LoadMap(root)
	if m == nil {
		return s
	}
	var missing, extra []string
	for _, f := range b.Folders {
		if _, ok := m.Dirs[filepath.ToSlash(f)]; !ok {
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
