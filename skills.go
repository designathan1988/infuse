package main

// Skills, in Claude Code's format: a folder with SKILL.md (frontmatter name and description, then the
// instructions) and any files it refers to. Found in .infuse/skills and .claude/skills of the project and
// in ~/.infuse/skills and ~/.claude/skills. The agent sees each skill's name and description; the Skill
// tool loads the instructions when the task matches.

import (
	"context"
	"encoding/json"
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

func skillDirs(root string) [][2]string {
	home, _ := os.UserHomeDir()
	return [][2]string{
		{filepath.Join(root, infuseDirName, "skills"), "project"},
		{filepath.Join(root, ".claude", "skills"), "project"},
		{filepath.Join(InfuseHome(), "skills"), "global"},
		{filepath.Join(home, ".claude", "skills"), "global"},
	}
}

// parseSkill reads the frontmatter (name, description) and the body of a SKILL.md.
func parseSkill(text string) (name, desc, body string) {
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
	for _, d := range skillDirs(root) {
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
			name, desc, _ := parseSkill(string(b))
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

func skillsPrompt(root string) string {
	var b strings.Builder
	for _, s := range LoadSkills(root) {
		b.WriteString("- " + s.Name + ": " + s.Description + "\n")
	}
	return b.String()
}

type skillTool struct{}

func (skillTool) Category() string { return "read" }
func (skillTool) Spec() ToolSpec {
	return ToolSpec{Name: "Skill", Description: "Loads the instructions of a skill (listed in the system prompt) when the task matches its description. Returns the SKILL.md and the skill's folder, whose files you can read.",
		InputSchema: schema(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)}
}

func (skillTool) Run(ctx context.Context, tc *ToolContext, input json.RawMessage) (string, bool) {
	var in struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(input, &in)
	for _, s := range LoadSkills(tc.S.Root) {
		if strings.EqualFold(s.Name, in.Name) {
			b, err := os.ReadFile(filepath.Join(s.Dir, "SKILL.md"))
			if err != nil {
				return err.Error(), true
			}
			_, _, body := parseSkill(string(b))
			tc.S.markRead(filepath.Join(s.Dir, "SKILL.md"))
			return tc.Say(M("skill.head", "name", s.Name, "folder", s.Dir)) + "\n\n" + tc.clip(body), false
		}
	}
	return tc.Say(M("skill.not_found", "name", in.Name)), true
}

func init() { registerTool(skillTool{}) }
