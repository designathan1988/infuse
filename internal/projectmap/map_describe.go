package projectmap

// DescribeDir: short, agent-facing description of one directory — what is infused into `ls` output.

import (
	"fmt"
	"strings"
	"time"
)

func DescribeDir(m *ProjectMap, relDir string) string {
	if relDir == "" {
		relDir = "."
	}
	d, ok := m.Dirs[relDir]
	if !ok {
		return ""
	}
	name := d.Path
	if name == "." {
		name = "project root"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[infuse] %s: %d files, %d dirs, %d lines", name, len(d.Files), len(d.Subdirs), d.Lines)
	if m.GeneratedAt > 0 {
		fmt.Fprintf(&b, " (map generated %s; LS again after the tree changes)", time.UnixMilli(m.GeneratedAt).Format("2006-01-02 15:04"))
	}
	for _, fn := range d.Files {
		key := fn
		if d.Path != "." {
			key = d.Path + "/" + fn
		}
		f, ok := m.Files[key]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n  %s (%dl)", fn, f.Lines)
		if len(f.Exports) > 0 {
			ex := f.Exports
			more := ""
			if len(ex) > 8 {
				ex, more = ex[:8], "…"
			}
			fmt.Fprintf(&b, " exports %s%s", strings.Join(ex, ", "), more)
		}
		if len(f.Dependents) > 0 {
			fmt.Fprintf(&b, " ← used by %d", len(f.Dependents))
		}
		if f.Lines >= hotspotLines {
			b.WriteString(" ⚠ hotspot")
		}
	}
	for _, s := range d.Subdirs {
		key := s
		if d.Path != "." {
			key = d.Path + "/" + s
		}
		if sd, ok := m.Dirs[key]; ok {
			fmt.Fprintf(&b, "\n  %s/ (%dl, %d files)", s, sd.Lines, len(sd.Files))
		}
	}
	return b.String()
}
