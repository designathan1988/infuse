package agent

// Output styles: how the agent writes, chosen by the outputStyle setting. The catalog is a data table;
// the user adds styles as .infuse/output-styles/*.md (frontmatter name and description, the body is the
// instruction), in the project or in ~/.infuse. A project style hides a global one of the same name.

import (
	"encoding/json"
	"infuse/internal/data"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"strings"
)

// OutputStyle is one row of the catalog or one user file.
type OutputStyle struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`       // a catalog id, for the built-in styles
	Description string `json:"description,omitempty"` // the user's own words, for their files
	Prompt      string `json:"prompt"`
	Source      string `json:"source"` // builtin, or the file
}

func builtinOutputStyles() []OutputStyle {
	var out []OutputStyle
	if err := json.Unmarshal(data.Read("output_styles.json"), &out); err != nil {
		panic("infuse: output_styles.json: " + err.Error())
	}
	for i := range out {
		out[i].Source = "builtin"
	}
	return out
}

func outputStyleDirs(root string) []string {
	dirs := []string{filepath.Join(root, infuseDirName, "output-styles")}
	dirs = append(dirs, pluginDirs(root, "output-styles")...)
	return append(dirs, filepath.Join(InfuseHome(), "output-styles"))
}

// OutputStyles lists the catalog and the user's styles; a user file may refine a built-in by name.
func OutputStyles(root string) []OutputStyle {
	out := builtinOutputStyles()
	seen := map[string]bool{}
	for _, dir := range outputStyleDirs(root) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			st, ok := readOutputStyle(filepath.Join(dir, e.Name()))
			if !ok || seen[st.Name] {
				continue
			}
			seen[st.Name] = true
			out = upsertStyle(out, st)
		}
	}
	return out
}

func readOutputStyle(p string) (OutputStyle, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return OutputStyle{}, false
	}
	fm, body := knowledge.ParseFrontmatter(string(b))
	name := firstNonEmpty(fm.Value("name"), strings.TrimSuffix(filepath.Base(p), ".md"))
	return OutputStyle{Name: name, Description: fm.Value("description"), Prompt: strings.TrimSpace(body), Source: p}, true
}

func upsertStyle(list []OutputStyle, st OutputStyle) []OutputStyle {
	for i := range list {
		if list[i].Name == st.Name {
			list[i] = st
			return list
		}
	}
	return append(list, st)
}

// outputStylePrompt is the instruction of the session's style ("" for the default). A style that no
// longer exists adds nothing.
func (s *Session) outputStylePrompt() string {
	name := s.settings.OutputStyle
	if name == "" {
		return ""
	}
	for _, st := range OutputStyles(s.Root) {
		if st.Name == name {
			return st.Prompt
		}
	}
	return ""
}
