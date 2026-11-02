package knowledge

// The grounds as one document, for an external agent: what infuse knows of the project, in the file
// that agent reads by itself. Generated from data, deterministic, regenerated at each launch.

import (
	"fmt"
	"infuse/internal/base"
	"infuse/internal/journal"
	"infuse/internal/oracle"
	"infuse/internal/projectmap"
	"strings"
)

// BlockGrounds names the launcher's block in the agent's instruction file.
const BlockGrounds = "grounds"

const groundsLevelRows = 12

// Grounds states the map, the base, the oracle, the lessons, the slow commands and the installed
// versions.
func Grounds(root string) string {
	var b strings.Builder
	b.WriteString("# Project grounds (infuse)\n\nGenerated from the code and the project's records before this agent opened.\n")
	writeGroundsMap(&b, root)
	if bc := base.Load(root); bc != nil {
		b.WriteString("\n## Architecture base\n" + bc.Contract())
	}
	writeGroundsOracle(&b, root)
	if l := journal.LessonsBrief(root); l != "" {
		b.WriteString("\n## Bad habits already seen in this project\n" + l + "\n")
	}
	if slow := journal.SlowCommands(root, 6); len(slow) > 0 {
		b.WriteString("\n## Slow commands (measured)\n")
		for _, c := range slow {
			b.WriteString("- " + c.CostLine() + "\n")
		}
	}
	if v := InstalledPrompt(root, 30); v != "" {
		b.WriteString("\n## Installed versions (their documentation, not what you remember)\n" + v)
	}
	return b.String()
}

func writeGroundsMap(b *strings.Builder, root string) {
	m := projectmap.Peek(root)
	if m == nil {
		return
	}
	b.WriteString("\n## Project map (the rest in .infuse/map/)\n")
	b.WriteString(projectmap.StableDir(m, ".") + "\n")
	for i, h := range m.Hotspots {
		if i >= 8 {
			break
		}
		fmt.Fprintf(b, "- too large: %s (%d lines)\n", h.Path, h.Lines)
	}
	b.WriteString(projectmap.LevelsBrief(m, groundsLevelRows))
}

func writeGroundsOracle(b *strings.Builder, root string) {
	spec, err := oracle.LoadOracleSpec(root)
	if err != nil || len(spec.Checks) == 0 {
		return
	}
	b.WriteString("\n## Definition of done (.infuse/oracle/checks.json; read-only, run by the executor)\n")
	for _, c := range spec.Checks {
		text := c.Description
		if text == "" {
			text = c.Kind
		}
		fmt.Fprintf(b, "- %s: %s\n", c.ID, text)
	}
}
