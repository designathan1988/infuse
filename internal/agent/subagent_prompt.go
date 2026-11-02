package agent

// The system prompt of every subagent: its role plus the project's grounds, never only the task.

import (
	"fmt"
	"strings"
)

func subagentSystem(s *Session, kind SubagentKind) string {
	var b strings.Builder
	b.WriteString(kind.Role)
	fmt.Fprintf(&b, "Project directory: %s.\n", s.workRoot())
	b.WriteString(responseLanguageRule(s.uiLang()) + "\n")
	if g := s.groundsBrief(); g != "" {
		b.WriteString("\n## Project grounds (the filter's knowledge)\n" + g + "\n")
	}
	return b.String()
}
