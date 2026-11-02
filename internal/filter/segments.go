package filter

import "strings"

// MaskHeredocs blanks heredoc bodies: document text never reads as a command.
func MaskHeredocs(cmd string) string { return maskHeredocBodies(cmd, FindHeredocs(cmd)) }

// CommandSegments lists the simple commands of a line the way the guard reads them: quoted text stays
// in one piece and heredoc bodies are gone.
func CommandSegments(cmd string) []string {
	var out []string
	for _, sg := range splitSegments(MaskHeredocs(cmd)) {
		if t := strings.TrimSpace(sg.text); t != "" {
			out = append(out, t)
		}
	}
	return out
}
