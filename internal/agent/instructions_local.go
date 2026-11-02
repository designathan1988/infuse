package agent

// Folder-level instruction files load when work enters the folder: Read and Write tell the agent the
// rules of the folder the file lives in, and Bash those of the folder the shell moved to, once per
// session. The root files are already in the prompt.

import (
	"os"
	"path/filepath"
	"strings"
)

var localInstructionNames = []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", ".cursorrules"}

func (s *Session) localInstructions(dir string) string {
	rel, err := filepath.Rel(s.Root, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	var parts []string
	cur := s.Root
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == "" || seg == "." {
			continue
		}
		cur = filepath.Join(cur, seg)
		for _, name := range localInstructionNames {
			p := filepath.Join(cur, name)
			s.Mu.Lock()
			seen := s.localInstrSeen[p]
			s.Mu.Unlock()
			if seen {
				continue
			}
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			s.Mu.Lock()
			if s.localInstrSeen == nil {
				s.localInstrSeen = map[string]bool{}
			}
			s.localInstrSeen[p] = true
			s.Mu.Unlock()
			rp, _ := filepath.Rel(s.Root, p)
			parts = append(parts, "<!-- "+filepath.ToSlash(rp)+" -->\n"+string(b))
		}
	}
	return strings.Join(parts, "\n\n")
}
