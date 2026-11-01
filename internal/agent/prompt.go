package agent

// System prompt assembly: identity, environment, the project's knowledge (map, rules, lessons, oracle),
// and layered instructions kept for compatibility with existing CLAUDE.md / AGENTS.md files.

import (
	"fmt"
	"infuse/internal/journal"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func buildSystemPrompt(s *Session, rules *RuleSet) string {
	var b strings.Builder
	b.WriteString(responseLanguageRule(s.uiLang()) + "\n\n")
	writePromptIdentity(&b)
	writePromptEnvironment(&b, s)
	// The project's knowledge — the grounds of the filter.
	writePromptGrounds(&b, s, rules)
	writePromptInstructions(&b, s)
	return b.String()
}

func writePromptIdentity(b *strings.Builder) {
	b.WriteString("You are the software engineering agent of infuse Studio, working directly in the user's project with the tools available.\n\n")
	b.WriteString("Working principles:\n")
	b.WriteString("- Understand before acting: read the files involved and the project map; identify dependents before changing anything.\n")
	b.WriteString("- Build within the project's existing structure. Do not scatter responsibilities; do not add conditionals where an extension point exists.\n")
	b.WriteString("- Minimal, precise changes: use Edit for targeted changes; do not rewrite working files without need; do not remove validations or safety checks.\n")
	b.WriteString("- Never modify tests or criteria to make them pass; never silence errors (empty catch blocks, default values that hide failures).\n")
	b.WriteString("- Verify by running (build, tests, the program itself) and report the real result. If you cannot reach the goal, say exactly what you tried, the evidence and where you stopped — that is a valid result.\n")
	b.WriteString("- Consult the official documentation of the installed version before trial and error.\n")
	b.WriteString("- Answer directly.\n\n")

	b.WriteString("infuse protocol (enforced by infuse on every action; an action outside it is refused with the reason):\n")
	b.WriteString("1. Before changing files: record the plan with TodoWrite, each open task with a proof.\n")
	b.WriteString("2. Before writing code that uses an external library or API: consult the official documentation of the version in use during this turn (WebSearch, WebFetch, pinned documentation, the package's code).\n")
	b.WriteString("3. The code follows the project's conventions and the file size limit.\n")
	b.WriteString("4. Two failures in a row require investigation before another attempt.\n")
	b.WriteString("5. The turn only ends with the plan completed (done or blocked), the project's checker reporting no errors in the changed files, a checked Report, and the findings of the independent review addressed.\n\n")
}

func writePromptEnvironment(b *strings.Builder, s *Session) {
	fmt.Fprintf(b, "Environment: %s/%s · date %s · project directory: %s · current directory: %s\n", runtime.GOOS, runtime.GOARCH, time.Now().Format("2006-01-02"), s.Root, s.Cwd)
	if branch := gitOutput(s.Root, "rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		fmt.Fprintf(b, "Git: branch %s", branch)
		if st := gitOutput(s.Root, "status", "--porcelain"); st != "" {
			fmt.Fprintf(b, " · %d file(s) with changes", len(strings.Split(strings.TrimSpace(st), "\n")))
		}
		b.WriteString("\n")
	}
	if s.mode() == "plan" {
		b.WriteString("PLAN MODE: you may read and investigate, but writing and execution are disabled. Produce a plan for approval.\n")
	}
}

func writePromptGrounds(b *strings.Builder, s *Session, rules *RuleSet) {
	if m := LoadOrBuildMap(s.Root); m != nil {
		b.WriteString("\n## Project map (generated from the code)\n")
		b.WriteString(DescribeDir(m, ".") + "\n")
		if len(m.Hotspots) > 0 {
			b.WriteString("Files that are too large (avoid adding responsibilities to them):\n")
			for i, h := range m.Hotspots {
				if i >= 8 {
					break
				}
				fmt.Fprintf(b, "- %s (%d lines)\n", h.Path, h.Lines)
			}
		}
		b.WriteString("Use LS on any folder to see its map; when you write a file you are told what depends on it.\n")
	}
	if bg := baseGrounds(s.Root); bg != "" {
		b.WriteString("\n## Architecture base (chosen by the user; code outside it is refused)\n" + bg + "\n")
	}
	if oracle := readOracle(s.Root); oracle != "" {
		b.WriteString("\n## Definition of done (set by the user; read-only — you cannot change it)\n" + oracle + "\n")
	}
	if lines := alwaysRuleTexts(s.Root, rules); len(lines) > 0 {
		b.WriteString("\n## Project rules\n" + strings.Join(lines, "\n") + "\n")
	}
	if l := lessonsSummary(s.Root); l != "" {
		b.WriteString("\n## Bad habits already seen in this project (do not repeat them)\n" + l + "\n")
	}
}

func writePromptInstructions(b *strings.Builder, s *Session) {
	if lock := instructionsPrompt(s.Root); lock != "" {
		b.WriteString(lock + "\n")
	}
	if inst := loadInstructions(s.Root); inst != "" {
		b.WriteString("\n## User and project instructions\n" + inst + "\n")
	}
	if mem := readIfExists(filepath.Join(s.Root, infuseDirName, "memory.md")); mem != "" {
		b.WriteString("\n## Project memory\n" + mem + "\n")
	}
	if c := conventionsPrompt(s.Root); c != "" {
		b.WriteString("\n## Project conventions (follow them when writing code; .infuse/conventions.md)\n" + c + "\n")
	}
	if d := docsPrompt(s.Root); d != "" {
		b.WriteString("\n## Documentation pinned by the user (read it with Read before using these libraries)\n" + d)
	}
	if sk := skillsPrompt(s.Root); sk != "" {
		b.WriteString("\n## Available skills (load one with the Skill tool when the task matches it)\n" + sk)
	}
	if items := memoryPrompt(s); items != "" {
		b.WriteString("\n## Structured memory (most important first; record and remove items with the Memory tool)\n" + items)
	}
}

func gitOutput(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	noConsoleWindow(cmd)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func readIfExists(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readOracle(root string) string {
	dir := filepath.Join(root, infuseDirName, "oracle")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var parts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if t := readIfExists(filepath.Join(dir, e.Name())); t != "" {
			parts = append(parts, "### "+e.Name()+"\n"+t)
		}
	}
	return strings.Join(parts, "\n\n")
}

func alwaysRuleTexts(root string, set *RuleSet) []string {
	var out []string
	for _, r := range set.Rules {
		if r.When != "always" {
			continue
		}
		out = append(out, r.Expand(root)...)
	}
	return out
}

func lessonsSummary(root string) string {
	list := readJSONL(filepath.Join(root, infuseDirName, "lessons.jsonl"))
	if len(list) == 0 {
		return ""
	}
	count := map[string]int{}
	for _, e := range list {
		if v, ok := e["vice"].(string); ok {
			count[v]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var kvs []kv
	for k, v := range count {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].v > kvs[j].v })
	var parts []string
	for i, e := range kvs {
		if i >= 8 {
			break
		}
		signature, remedy := journal.Vice(e.k)
		line := fmt.Sprintf("- %s (%d time(s))", e.k, e.v)
		if remedy != "" {
			line += "\n  signature: " + signature + "\n  remedy: " + remedy
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n")
}

// groundsBrief is the project's knowledge in a bounded form, for the independent reviewer, the
// subagents and the mission auditor: the same grounds the agent gets, never only the diff.
func (s *Session) groundsBrief() string {
	var b strings.Builder
	if m := LoadMap(s.Root); m != nil {
		if d := DescribeDir(m, "."); d != "" {
			b.WriteString("### Project map\n" + truncate(d, 3000) + "\n\n")
		}
	}
	if c := conventionsPrompt(s.Root); c != "" {
		b.WriteString("### Project conventions\n" + truncate(c, 2000) + "\n\n")
	}
	if l := lessonsSummary(s.Root); l != "" {
		b.WriteString("### Lessons from this project\n" + l + "\n\n")
	}
	if k := knowledgePrompt(s.Root); k != "" {
		b.WriteString("### Facts read from the project's documents\n" + truncate(k, 4000) + "\n")
	}
	return b.String()
}

// loadInstructions: global, then project; compatible with CLAUDE.md and AGENTS.md.
func loadInstructions(root string) string {
	var parts []string
	for _, p := range []string{
		filepath.Join(InfuseHome(), "instructions.md"),
		filepath.Join(homeDir(), ".claude", "CLAUDE.md"),
		filepath.Join(root, infuseDirName, "instructions.md"),
		filepath.Join(root, "CLAUDE.md"),
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, "GEMINI.md"),
		filepath.Join(root, ".cursorrules"),
		filepath.Join(root, ".github", "copilot-instructions.md"),
		filepath.Join(root, ".claude", "CLAUDE.md"),
	} {
		t := readIfExists(p)
		if t == "" && strings.HasPrefix(p, root) { // deleted from the working tree: the committed version still rules
			if rel, err := filepath.Rel(root, p); err == nil && contains(loadProjectRules(root).Missing, filepath.ToSlash(rel)) {
				t = gitHeadFile(root, filepath.ToSlash(rel))
			}
		}
		if t != "" {
			parts = append(parts, "<!-- "+p+" -->\n"+resolveInstructionImports(root, filepath.Dir(p), t, 0))
		}
	}
	return strings.Join(parts, "\n\n")
}
