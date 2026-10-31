package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// Search across sessions.

type sessionHit struct {
	ID      string `json:"id"`
	Root    string `json:"root"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

// searchSessions finds q (case-insensitive) in the titles and messages of every project's sessions.
func searchSessions(q string, max int) []sessionHit {
	q = strings.ToLower(strings.TrimSpace(q))
	out := []sessionHit{}
	if len([]rune(q)) < 2 {
		return out
	}
	for _, p := range loadProjects() {
		entries, _ := os.ReadDir(sessionsDir(p.Root))
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".json") || len(out) >= max {
				continue
			}
			var d SessionData
			if readJSON(filepath.Join(sessionsDir(p.Root), e.Name()), &d) != nil {
				continue
			}
			hit := ""
			if strings.Contains(strings.ToLower(d.Title), q) {
				hit = d.Title
			}
			for i := len(d.Messages) - 1; i >= 0 && hit == ""; i-- {
				for _, b := range d.Messages[i].Content {
					text := b.Text
					if b.Type == "tool_result" {
						text = b.Content
					}
					low := strings.ToLower(text)
					if k := strings.Index(low, q); k >= 0 {
						hit = snippetAround(text, k, len(q))
						break
					}
				}
			}
			if hit != "" {
				out = append(out, sessionHit{ID: d.ID, Root: d.Root, Title: d.Title, Snippet: hit})
			}
		}
	}
	return out
}

// snippetAround cuts ~40 characters on each side of the match at byte k (rune-safe).
