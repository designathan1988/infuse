package agent

// The architecture base is the user's structure: code written outside it is refused before disk.

import (
	"infuse/internal/base"
)

func (s *Session) baseViolation(rel string, isCode bool) Msg {
	if name := base.CheckWrite(s.Root, rel, isCode); name != "" {
		return M("protocol.out_of_base", "file", rel, "base", name)
	}
	return Msg{}
}

// baseGrounds is the base as the prompt states it.
func baseGrounds(root string) string {
	b := base.Load(root)
	if b == nil {
		return ""
	}
	return b.Summary()
}
