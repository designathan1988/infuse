// Package knowledge holds what the project already knows: instructions, conventions, documents,
// structured memory and skills. It never runs the agent; it reads, compiles and formats knowledge.
package knowledge

// Skills, in Claude Code's format: a folder with SKILL.md (frontmatter name and description, then the
// instructions) and any files it refers to. Found in .infuse/skills and .claude/skills of the project and
// in ~/.infuse/skills and ~/.claude/skills.

import (
	"infuse/internal/core"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Dir         string `json:"dir"`
	Scope       string `json:"scope"`
	Body        string `json:"body,omitempty"`
}

func SkillDirs(root string) [][2]string {
	home, _ := os.UserHomeDir()
	dirs := [][2]string{
		{filepath.Join(root, core.InfuseDirName, "skills"), "project"},
		{filepath.Join(root, ".claude", "skills"), "project"},
	}
	for _, d := range PluginContentDirs(root, "skills") {
		dirs = append(dirs, [2]string{d, "plugin"})
	}
	return append(dirs, [2]string{filepath.Join(core.InfuseHome(), "skills"), "global"}, [2]string{filepath.Join(home, ".claude", "skills"), "global"})
}

// PluginContentDirs lists one kind of content (agents, commands, skills) of the installed plugins.
func PluginContentDirs(root, kind string) []string {
	base := filepath.Join(root, core.InfuseDirName, "plugins")
	entries, _ := os.ReadDir(base)
	var out []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, filepath.Join(base, e.Name(), kind))
		}
	}
	return out
}

// ParseSkill reads the frontmatter (name, description) and the body of a SKILL.md.
func ParseSkill(text string) (name, desc, body string) {
	body = text
	t := strings.ReplaceAll(text, "\r\n", "\n")
	if strings.HasPrefix(t, "---\n") {
		if end := strings.Index(t[4:], "\n---"); end >= 0 {
			for _, line := range strings.Split(t[4:4+end], "\n") {
				k, v, ok := strings.Cut(line, ":")
				if !ok {
					continue
				}
				v = strings.Trim(strings.TrimSpace(v), `"'`)
				switch strings.TrimSpace(k) {
				case "name":
					name = v
				case "description":
					desc = v
				}
			}
			body = strings.TrimLeft(t[4+end+4:], "\n")
		}
	}
	return
}

// LoadSkills lists the skills; a project skill hides a global one with the same name.
func LoadSkills(root string) []Skill {
	seen := map[string]bool{}
	var out []Skill
	for _, d := range SkillDirs(root) {
		entries, _ := os.ReadDir(d[0])
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(d[0], e.Name())
			b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
			if err != nil {
				continue
			}
			name, desc, _ := ParseSkill(string(b))
			if name == "" {
				name = e.Name()
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, Skill{Name: name, Description: desc, Dir: dir, Scope: d[1]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func SkillsPrompt(root string) string {
	var b strings.Builder
	for _, s := range LoadSkills(root) {
		b.WriteString("- " + s.Name + ": " + s.Description + "\n")
	}
	return b.String()
}
