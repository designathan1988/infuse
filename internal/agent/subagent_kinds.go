package agent

// Subagent kinds, as a table: the built-in ones and the user's, defined in .infuse/agents/*.md with a
// frontmatter of name, description, tools and model; the body is the kind's role. A kind only narrows
// what a subagent may call: every call still goes through the session's permissions and the filter.

import (
	"encoding/json"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SubagentKind is one type of subagent.
type SubagentKind struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tools       []string `json:"tools,omitempty"` // nil = every tool a subagent may have; a trailing * is a prefix
	Model       string   `json:"model,omitempty"` // "" or inherit = the session's; fast = the fast model
	ReadOnly    bool     `json:"readOnly"`        // works side by side with others of its reply
	Role        string   `json:"-"`
	Source      string   `json:"source"` // builtin, or the definition file
}

var readOnlyTools = map[string]bool{"Read": true, "Glob": true, "Grep": true, "LS": true, "WebFetch": true, "WebSearch": true}

// neverSubagent are the session's own controls: a subagent cannot spawn agents or leave plan mode.
var neverSubagent = map[string]bool{"Agent": true, "EnterPlanMode": true, "ExitPlanMode": true}

func readOnlyNames() []string {
	names := make([]string, 0, len(readOnlyTools))
	for n := range readOnlyTools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The Studio spec gives subagents the fast model; the reviewer keeps the session's, its verdict is a gate.
var builtinKinds = []SubagentKind{
	{Name: "explore", Description: "research and reading in the project (read-only)", Tools: readOnlyNames(), Model: "fast", ReadOnly: true, Source: "builtin",
		Role: "You are a research subagent in a software project. Read and search whatever is needed to answer the task and return an objective conclusion, with file paths and lines. You only have read tools.\n"},
	{Name: "review", Description: "independent reviewer of the current changes (sees the diff, the plan and the criteria, not your reasoning): use it before declaring a piece of work done", Tools: readOnlyNames(), ReadOnly: true, Source: "builtin",
		Role: "You are an independent code reviewer. You did not write this work and you have not seen the reasoning of whoever wrote it. Check the changes against the request, the plan and the criteria: look for logic errors, unhandled edge cases, removed validations or checks, tests changed so they pass, code that is never called, scattered responsibilities. Read whatever files you need. Answer with a list of findings, from most to least severe, each with file:line, the problem and its concrete consequence; and finally a verdict: approve or fix. If there are no findings, say so.\n"},
	{Name: "general", Description: "a bounded task with any tool", Model: "fast", Source: "builtin",
		Role: "You are a subagent carrying out a bounded task in a software project. Do only what the task asks and return a short account of what you did, with the evidence (commands and results).\n"},
}

// allows reports whether a subagent of this kind may call the tool.
func (k SubagentKind) allows(tool string) bool {
	if neverSubagent[tool] {
		return false
	}
	if k.Tools == nil {
		return true
	}
	for _, t := range k.Tools {
		if t == tool || strings.HasSuffix(t, "*") && strings.HasPrefix(tool, strings.TrimSuffix(t, "*")) {
			return true
		}
	}
	return false
}

func subagentAllows(kind, tool string) bool { return builtinKind(kind).allows(tool) }

func builtinKind(name string) SubagentKind {
	for _, k := range builtinKinds {
		if k.Name == name {
			return k
		}
	}
	return builtinKinds[len(builtinKinds)-1] // general
}

var reKindName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// agentDirs are where user kinds live, most specific first: a project definition hides a global one.
func agentDirs(root string) []string {
	dirs := []string{filepath.Join(root, infuseDirName, "agents")}
	dirs = append(dirs, pluginDirs(root, "agents")...)
	return append(dirs, filepath.Join(InfuseHome(), "agents"))
}

// loadUserKinds reads the definitions; a built-in name cannot be taken over.
func loadUserKinds(root string) []SubagentKind {
	seen := map[string]bool{}
	for _, k := range builtinKinds {
		seen[k.Name] = true
	}
	var out []SubagentKind
	for _, dir := range agentDirs(root) {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			k, ok := parseKind(filepath.Join(dir, e.Name()))
			if !ok || seen[k.Name] {
				continue
			}
			seen[k.Name] = true
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parseKind(p string) (SubagentKind, bool) {
	b, err := os.ReadFile(p)
	if err != nil {
		return SubagentKind{}, false
	}
	fm, body := knowledge.ParseFrontmatter(string(b))
	k := SubagentKind{Name: fm.Value("name"), Description: fm.Value("description"), Tools: fm["tools"], Model: fm.Value("model"), Role: strings.TrimSpace(body) + "\n", Source: p}
	if k.Name == "" {
		k.Name = strings.TrimSuffix(filepath.Base(p), ".md")
	}
	if !reKindName.MatchString(k.Name) {
		return SubagentKind{}, false
	}
	k.ReadOnly = k.Tools != nil
	for _, t := range k.Tools {
		k.ReadOnly = k.ReadOnly && readOnlyTools[t]
	}
	return k, true
}

// SubagentKinds is every kind the project's subagents can be.
func SubagentKinds(root string) []SubagentKind {
	return append(append([]SubagentKind{}, builtinKinds...), loadUserKinds(root)...)
}

// subagentKind finds a kind by name; "" is general.
func (s *Session) subagentKind(name string) (SubagentKind, bool) {
	if name == "" {
		return builtinKind("general"), true
	}
	for _, k := range SubagentKinds(s.Root) {
		if k.Name == name {
			return k, true
		}
	}
	return SubagentKind{}, false
}

// subagentModel is the model a kind runs on. A model the session's connection does not serve would
// fail every request, so the kind then runs on the session's own.
func (s *Session) subagentModel(k SubagentKind) string {
	switch k.Model {
	case "", "inherit":
		return s.Model
	case "fast":
		return s.fastModel()
	}
	models := s.settings.Connections[s.providerID()].Models
	if len(models) == 0 || contains(models, k.Model) {
		return k.Model
	}
	return s.Model
}

// agentToolSchema is the Agent tool's input, with the kinds as its enum.
func agentToolSchema(kinds []SubagentKind) json.RawMessage {
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, k.Name)
	}
	enum, _ := json.Marshal(names)
	return schema(`{"type":"object","properties":{"kind":{"type":"string","enum":` + string(enum) + `},"description":{"type":"string","description":"short title"},"prompt":{"type":"string","description":"the full task: the subagent does not see this conversation"}},"required":["kind","prompt"]}`)
}

// agentToolDescription lists the kinds for the model.
func agentToolDescription(kinds []SubagentKind) string {
	var b strings.Builder
	b.WriteString("Delegates a task to a subagent with a clean context, which returns only its conclusion. Several read-only subagents called in the same reply work at the same time (a team): split independent research among them.")
	for _, k := range kinds {
		b.WriteString(" kind=" + k.Name + ": " + k.Description + ".")
	}
	b.WriteString(" The subagent's actions go through the same permissions and the filter.")
	return b.String()
}

// withAgentKinds shows the project's own kinds in the Agent tool's spec.
func (s *Session) withAgentKinds(specs []ToolSpec) []ToolSpec {
	user := loadUserKinds(s.Root)
	if len(user) == 0 {
		return specs
	}
	kinds := append(append([]SubagentKind{}, builtinKinds...), user...)
	for i := range specs {
		if specs[i].Name == "Agent" {
			specs[i].Description, specs[i].InputSchema = agentToolDescription(kinds), agentToolSchema(kinds)
		}
	}
	return specs
}
