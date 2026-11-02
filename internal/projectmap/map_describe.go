package projectmap

// DescribeDir: short, agent-facing description of one directory — what is infused into `ls` output.

import (
	"fmt"
	"path"
	"strings"
	"time"
)

// DescribeDir is the live description: it states when the map was generated.
func DescribeDir(m *ProjectMap, relDir string) string { return describe(m, relDir, true) }

// StableDir is the stored description: the same text as long as the code is the same.
func StableDir(m *ProjectMap, relDir string) string { return describe(m, relDir, false) }

// describe writes one folder; the stored documents leave the clock out, so they only change when the
// code does.
func describe(m *ProjectMap, relDir string, live bool) string {
	if relDir == "" {
		relDir = "."
	}
	d, ok := m.Dirs[relDir]
	if !ok {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[infuse] %s: %d files, %d dirs, %d lines", dirTitle(d), len(d.Files), len(d.Subdirs), d.Lines)
	if live && m.GeneratedAt > 0 {
		fmt.Fprintf(&b, " (map generated %s; LS again after the tree changes)", time.UnixMilli(m.GeneratedAt).Format("2006-01-02 15:04"))
	}
	if d.Role != "" && !d.Intended {
		fmt.Fprintf(&b, "\n  role: %s", d.Role)
	}
	writeFileLines(&b, m, d, live)
	for _, s := range d.Subdirs {
		if sd, ok := m.Dirs[join(d.Path, s)]; ok {
			fmt.Fprintf(&b, "\n  %s/ (%dl, %d files)", s, sd.Lines, len(sd.Files))
		}
	}
	writeDirLevels(&b, m, d)
	return b.String()
}

func dirTitle(d *DirInfo) string {
	name := d.Path
	if name == "." {
		name = "project root"
	}
	if !d.Intended {
		return name
	}
	if d.Role != "" {
		return name + fmt.Sprintf(" (intended, no code yet — %s)", d.Role)
	}
	return name + " (intended, no code yet)"
}

func writeFileLines(b *strings.Builder, m *ProjectMap, d *DirInfo, live bool) {
	limit := m.HotspotLines
	if limit <= 0 {
		limit = defaultHotspotLines
	}
	for _, fn := range d.Files {
		f, ok := m.Files[join(d.Path, fn)]
		if !ok || !live && isContextFile(fn) {
			continue
		}
		fmt.Fprintf(b, "\n  %s (%dl)", fn, f.Lines)
		if len(f.Exports) > 0 {
			ex := f.Exports
			more := ""
			if len(ex) > 8 {
				ex, more = ex[:8], "…"
			}
			fmt.Fprintf(b, " exports %s%s", strings.Join(ex, ", "), more)
		}
		if len(f.Dependents) > 0 {
			fmt.Fprintf(b, " ← used by %d", len(f.Dependents))
		}
		if f.Lines >= limit && !isGeneratedNoise(f.Path) {
			b.WriteString(" ⚠ hotspot")
		}
	}
}

// writeDirLevels adds what the upper levels of the map say about this folder.
func writeDirLevels(b *strings.Builder, m *ProjectMap, d *DirInfo) {
	for _, f := range m.Features {
		if len(f.Dirs) < 2 || !contains(f.Dirs, d.Path) {
			continue
		}
		fmt.Fprintf(b, "\n  feature %s: %d files across %s", f.Name, len(f.Files), strings.Join(firstOf(f.Dirs, 5), ", "))
	}
	for _, r := range m.Data {
		if path.Dir(r.File) == d.Path {
			fmt.Fprintf(b, "\n  data %s: %s", r.Name, ownerText(r))
		}
	}
	for _, e := range m.Examples {
		if path.Dir(e.File) == d.Path {
			fmt.Fprintf(b, "\n  how %s is done here: %s:%d", e.Label, e.File, e.Line)
		}
	}
	for _, x := range m.Drift {
		if x.Kind != "intended-only" && (x.Path == d.Path || path.Dir(x.Path) == d.Path) {
			fmt.Fprintf(b, "\n  drift %s: %s", x.Path, x.Detail)
		}
	}
}

func ownerText(r DataOwner) string {
	if r.Owner != "" {
		return "owner " + r.Owner
	}
	return fmt.Sprintf("no single owner, written by %d files: %s", len(r.Writers), strings.Join(firstOf(r.Writers, 4), ", "))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func firstOf(list []string, n int) []string {
	if len(list) > n {
		return list[:n]
	}
	return list
}

// LevelsBrief is the project-wide reading of the map, for the prompt and MAP.md: where each feature is
// split, who owns each datum, real examples of common operations, and where the code left the base. A
// positive limit bounds each section.
func LevelsBrief(m *ProjectMap, limit int) string {
	bound := func(n int) int {
		if limit > 0 && n > limit {
			return limit
		}
		return n
	}
	var b strings.Builder
	section := func(title string, n int, line func(i int) string) {
		if n == 0 {
			return
		}
		fmt.Fprintf(&b, "\n### %s\n", title)
		for i := 0; i < bound(n); i++ {
			b.WriteString("- " + line(i) + "\n")
		}
	}
	split := splitFeatures(m)
	section("Features split across folders", len(split), func(i int) string {
		return fmt.Sprintf("%s: %d files in %s", split[i].Name, len(split[i].Files), strings.Join(firstOf(split[i].Dirs, 6), ", "))
	})
	section("Data and their owners", len(m.Data), func(i int) string {
		r := m.Data[i]
		return fmt.Sprintf("%s (%s, %s:%d): %s", r.Name, r.Kind, r.File, r.Line, ownerText(r))
	})
	section("How it is done here", len(m.Examples), func(i int) string {
		e := m.Examples[i]
		return fmt.Sprintf("%s (%s): %s:%d `%s`", e.Label, e.Lang, e.File, e.Line, e.Text)
	})
	section("Intended vs. actual", len(m.Drift), func(i int) string {
		return fmt.Sprintf("%s: %s — %s", m.Drift[i].Kind, m.Drift[i].Path, m.Drift[i].Detail)
	})
	section("Exports nothing references", len(m.Dead), func(i int) string {
		return m.Dead[i].File + ": " + m.Dead[i].Symbol
	})
	return b.String()
}

func splitFeatures(m *ProjectMap) []Feature {
	var out []Feature
	for _, f := range m.Features {
		if len(f.Dirs) > 1 {
			out = append(out, f)
		}
	}
	return out
}
