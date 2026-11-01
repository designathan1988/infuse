package agent

// The system prompt of every subagent: its role plus the project's grounds, never only the task.

import (
	"fmt"
	"strings"
)

func subagentSystem(s *Session, kind string) string {
	var b strings.Builder
	switch kind {
	case "review":
		b.WriteString("You are an independent code reviewer. You did not write this work and you have not seen the reasoning of whoever wrote it. Check the changes against the request, the plan and the criteria: look for logic errors, unhandled edge cases, removed validations or checks, tests changed so they pass, code that is never called, scattered responsibilities. Read whatever files you need. Answer with a list of findings, from most to least severe, each with file:line, the problem and its concrete consequence; and finally a verdict: approve or fix. If there are no findings, say so.\n")
	case "explore":
		b.WriteString("You are a research subagent in a software project. Read and search whatever is needed to answer the task and return an objective conclusion, with file paths and lines. You only have read tools.\n")
	default:
		b.WriteString("You are a subagent carrying out a bounded task in a software project. Do only what the task asks and return a short account of what you did, with the evidence (commands and results).\n")
	}
	fmt.Fprintf(&b, "Project directory: %s.\n", s.workRoot())
	b.WriteString(responseLanguageRule(s.uiLang()) + "\n")
	if g := s.groundsBrief(); g != "" {
		b.WriteString("\n## Project grounds (the filter's knowledge)\n" + g + "\n")
	}
	return b.String()
}
