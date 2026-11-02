package journal

import (
	"fmt"
	"infuse/internal/core"
	"path/filepath"
	"sort"
	"strings"
)

// LessonsBrief is the vices seen in this project, most frequent first, each with its signature and
// remedy.
func LessonsBrief(root string) string {
	list := core.ReadJSONL(filepath.Join(root, core.InfuseDirName, "lessons.jsonl"))
	if len(list) == 0 {
		return ""
	}
	count := map[string]int{}
	for _, e := range list {
		if v, ok := e["vice"].(string); ok {
			count[v]++
		}
	}
	names := make([]string, 0, len(count))
	for k := range count {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if count[names[i]] != count[names[j]] {
			return count[names[i]] > count[names[j]]
		}
		return names[i] < names[j]
	})
	var parts []string
	for i, name := range names {
		if i >= 8 {
			break
		}
		signature, remedy := Vice(name)
		line := fmt.Sprintf("- %s (%d time(s))", name, count[name])
		if remedy != "" {
			line += "\n  signature: " + signature + "\n  remedy: " + remedy
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n")
}
