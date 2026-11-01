package agent

// The instruction summary the agent reads: what infuse extracted and will enforce.

import "strings"

// instructionsPrompt tells the agent what infuse extracted from the instructions and will enforce.
func instructionsPrompt(root string) string {
	r := loadProjectRules(root)
	if len(r.Files) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## What infuse enforces from the project's instructions\n")
	if r.Stale {
		b.WriteString("- " + M("instr.rules_stale").String() + "\n")
	}
	if len(r.Missing) > 0 {
		b.WriteString("- " + strings.Join(r.Missing, ", ") + " was deleted from the working folder; the git version, read above, is the one that applies. Do not delete instructions.\n")
	}
	if len(r.Required) > 0 {
		b.WriteString("- Required reading, in full, before changing files or running commands: " + strings.Join(r.Required, ", ") + ".\n")
	}
	if len(r.Reference) > 0 {
		b.WriteString("- Read-only reference; copying code from it or writing in it is forbidden: " + strings.Join(r.Reference, ", ") + ".\n")
	}
	if len(r.Commands) > 0 {
		b.WriteString("- The project's verification and measurement (use these, not tools of your own): " + strings.Join(r.Commands, " · ") + ".\n")
	}
	b.WriteString("- Repeating a measurement after each tweak, without researching the cause, blocks the turn until you study the documentation.\n")
	return b.String()
}
