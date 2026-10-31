package agent

// The Skill tool: the instructions of a skill listed in the system prompt, loaded on demand.

import (
	"context"
	"encoding/json"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"strings"
)

type Skill = knowledge.Skill

func LoadSkills(root string) []Skill      { return knowledge.LoadSkills(root) }
func skillsPrompt(root string) string     { return knowledge.SkillsPrompt(root) }
func parseSkill(text string) (string, string, string) { return knowledge.ParseSkill(text) }

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
	for _, s := range knowledge.LoadSkills(tc.S.Root) {
		if strings.EqualFold(s.Name, in.Name) {
			b, err := os.ReadFile(filepath.Join(s.Dir, "SKILL.md"))
			if err != nil {
				return err.Error(), true
			}
			_, _, body := knowledge.ParseSkill(string(b))
			tc.S.markRead(filepath.Join(s.Dir, "SKILL.md"))
			return tc.Say(M("skill.head", "name", s.Name, "folder", s.Dir)) + "\n\n" + tc.clip(body), false
		}
	}
	return tc.Say(M("skill.not_found", "name", in.Name)), true
}

func init() { registerTool(skillTool{}) }
