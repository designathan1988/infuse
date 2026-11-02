package filter

// User-defined rules — the part of the filter the user owns. Edited in the panel or by hand.
//
// Two files, merged in this order (global first, then project; later rules run later):
//   <home>/.infuse/rules.json      every project on this machine
//   <project>/.infuse/rules.json   this project only
//
//   when: "command"  match = regex on the command the agent is about to run; replace changes it
//   when: "write"    match = regex on the CONTENT of every file written during the command; replace rewrites it
//   when: "list"     match = regex on the directory path the agent is listing (posix, relative)
//   when: "always"   every command
//   say / infuse / infuseFile / vice / disable ("heredoc")

import (
	"fmt"
	"infuse/internal/core"
	"infuse/internal/journal"
	"infuse/internal/store"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Rule struct {
	ID         string  `json:"id,omitempty"`
	When       string  `json:"when"`
	Match      string  `json:"match,omitempty"`
	Flags      string  `json:"flags,omitempty"`
	Replace    *string `json:"replace,omitempty"`
	Say        string  `json:"say,omitempty"`
	Infuse     string  `json:"infuse,omitempty"`
	InfuseFile string  `json:"infuseFile,omitempty"`
	Vice       string  `json:"vice,omitempty"`
	Disable    string  `json:"disable,omitempty"`
	Enabled    *bool   `json:"enabled,omitempty"`

	problem core.Msg // infuse's own note in place of a rule (a rules file that could not be read)
}

type RulesFile struct {
	store.Versioned
	Rules   []Rule   `json:"rules"`
	Disable []string `json:"disable"`
}

type RuleSet struct {
	Rules    []Rule
	Disabled map[string]bool
	Sources  []string
}

func GlobalRulesFile() string { return filepath.Join(core.InfuseHome(), "rules.json") }
func ProjectRulesFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "rules.json")
}

// LoadRulesFile reads one rules file at the current schema version.
func LoadRulesFile(file string) (RulesFile, error) {
	rf := RulesFile{Rules: []Rule{}, Disable: []string{}}
	if err := core.ReadJSON(file, &rf); err != nil {
		return rf, err
	}
	_, _ = rf.Upgrade(nil)
	if rf.Rules == nil {
		rf.Rules = []Rule{}
	}
	if rf.Disable == nil {
		rf.Disable = []string{}
	}
	return rf, nil
}

// SaveRulesFile writes a rules file stamped with the current schema version.
func SaveRulesFile(file string, rf RulesFile) error {
	rf.SchemaVersion = store.Version
	return core.WriteJSON(file, rf)
}

func LoadRules(root string) *RuleSet {
	set := &RuleSet{Disabled: map[string]bool{}}
	for _, file := range []string{GlobalRulesFile(), ProjectRulesFile(root)} {
		if !core.Exists(file) {
			continue
		}
		rf, err := LoadRulesFile(file)
		if err != nil {
			set.Rules = append(set.Rules, Rule{When: "always", problem: core.M("rules.bad_json", "file", file, "error", err.Error())})
			continue
		}
		for _, r := range rf.Rules {
			if r.Enabled != nil && !*r.Enabled {
				continue
			}
			if r.Disable != "" {
				set.Disabled[r.Disable] = true
				continue
			}
			set.Rules = append(set.Rules, r)
		}
		for _, d := range rf.Disable {
			set.Disabled[d] = true
		}
		set.Sources = append(set.Sources, file)
	}
	return set
}

func (r Rule) regex() *regexp.Regexp {
	if r.Match == "" {
		return nil
	}
	flags := r.Flags
	if flags == "" {
		flags = "m"
	}
	prefix := "(?"
	for _, f := range flags {
		if f == 'i' || f == 'm' || f == 's' {
			prefix += string(f)
		}
	}
	prefix += ")"
	if prefix == "(?)" {
		prefix = ""
	}
	re, err := regexp.Compile(prefix + r.Match)
	if err != nil {
		return nil
	}
	return re
}

// jsReplacement converts $1-style JS replacement to Go's ${1} form.
var reDollar = regexp.MustCompile(`\$(\d+)`)

func jsReplacement(s string) string { return reDollar.ReplaceAllString(s, `$${$1}`) }

// expand is what a rule says, in English (expandMsgs gives the messages).
func (r Rule) Expand(root string) []string {
	return MsgStrings(r.expandMsgs(root, ""))
}

// expandMsgs is what a rule says: the user's own words as written, infuse's notes as messages. file is
// the written file a write rule matched ("" for the others).
func (r Rule) expandMsgs(root, file string) []core.Msg {
	var out []core.Msg
	if !r.problem.Empty() {
		out = append(out, r.problem)
	}
	if r.Say != "" {
		if file != "" {
			out = append(out, core.M("rules.say_in", "file", file, "text", r.Say))
		} else {
			out = append(out, core.M("rules.say", "text", r.Say))
		}
	}
	if r.Infuse != "" {
		out = append(out, core.M("text.verbatim", "text", r.Infuse))
	}
	if r.InfuseFile != "" {
		out = append(out, r.infusedFile(root, file))
	}
	return out
}

// infusedFile is the text of the rule's file. The file is read on the host and handed to the model, so
// it must be the project's or infuse's own, never a record: a rule cannot be a way to read the machine.
func (r Rule) infusedFile(root, file string) core.Msg {
	p := RealPath(CanonPath(root, r.InfuseFile))
	inPlace := core.Within(RealPath(root), p) || core.Within(RealPath(core.InfuseHome()), p)
	if !inPlace || containsRel(p, internalRels) {
		return core.M("rules.file_refused", "rule", r.ID, "path", r.InfuseFile)
	}
	if b, err := os.ReadFile(p); err == nil {
		return core.M("text.verbatim", "text", strings.TrimRight(string(b), " \t\r\n"))
	}
	if file != "" {
		return core.M("rules.file_missing_in", "file", file, "rule", r.ID, "path", r.InfuseFile)
	}
	return core.M("rules.file_missing", "rule", r.ID, "path", r.InfuseFile)
}

// msgStrings is the English of each message (for callers that only pass text to the model).
func MsgStrings(ms []core.Msg) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.String())
	}
	return out
}

// ApplyCommandRules may rewrite the command; returns the trailer lines rules produce (English).
func ApplyCommandRules(root string, set *RuleSet, command string) (string, []string) {
	cmd, ms := ApplyCommandRulesMsgs(root, set, command)
	return cmd, MsgStrings(ms)
}

// ApplyCommandRulesMsgs may rewrite the command; returns the messages rules produce.
func ApplyCommandRulesMsgs(root string, set *RuleSet, command string) (string, []core.Msg) {
	var lines []core.Msg
	cmd := command
	for _, r := range set.Rules {
		if r.When != "command" && r.When != "always" {
			continue
		}
		re := r.regex()
		if r.When == "command" && (re == nil || !re.MatchString(cmd)) {
			continue
		}
		if re != nil && r.Replace != nil {
			cmd = re.ReplaceAllString(cmd, jsReplacement(*r.Replace))
		}
		if r.Vice != "" {
			journal.Lesson(root, r.Vice, fmt.Sprintf("rule %s matched command: %s", r.ID, core.Truncate(command, 120)))
		}
		lines = append(lines, r.expandMsgs(root, "")...)
	}
	return cmd, lines
}

// ApplyWriteRules checks the files written during a command (absolute paths); English lines.
func ApplyWriteRules(root string, set *RuleSet, files []string) []string {
	return MsgStrings(ApplyWriteRulesMsgs(root, set, files))
}

// ApplyWriteRulesMsgs checks the files written during a command (absolute paths).
func ApplyWriteRulesMsgs(root string, set *RuleSet, files []string) []core.Msg {
	var lines []core.Msg
	var writeRules []Rule
	for _, r := range set.Rules {
		if r.When == "write" {
			writeRules = append(writeRules, r)
		}
	}
	if len(writeRules) == 0 {
		return nil
	}
	for _, file := range files {
		b, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		content := string(b)
		for _, r := range writeRules {
			re := r.regex()
			if re == nil || !re.MatchString(content) {
				continue
			}
			if r.Replace != nil {
				next := re.ReplaceAllString(content, jsReplacement(*r.Replace))
				if next != content {
					_ = os.WriteFile(file, []byte(next), 0o644)
					content = next
					lines = append(lines, core.M("rules.rewrote", "rule", r.ID, "file", file))
				}
			}
			if r.Vice != "" {
				journal.Lesson(root, r.Vice, fmt.Sprintf("rule %s matched in %s", r.ID, file))
			}
			lines = append(lines, r.expandMsgs(root, file)...)
		}
	}
	return lines
}

// ApplyListRules: rules for a directory the agent listed (posix path relative to root); English lines.
func ApplyListRules(root string, set *RuleSet, dir string) []string {
	return MsgStrings(ApplyListRulesMsgs(root, set, dir))
}

// ApplyListRulesMsgs: rules for a directory the agent listed (posix path relative to root).
func ApplyListRulesMsgs(root string, set *RuleSet, dir string) []core.Msg {
	var lines []core.Msg
	for _, r := range set.Rules {
		if r.When != "list" {
			continue
		}
		if re := r.regex(); re != nil && !re.MatchString(dir) {
			continue
		}
		lines = append(lines, r.expandMsgs(root, "")...)
	}
	return lines
}

var ExampleRules = RulesFile{
	Rules: []Rule{
		{ID: "no-swallowed-errors", When: "write", Match: `catch\s*(\([^)]*\))?\s*\{\s*\}`, Say: "empty catch block: the error is being swallowed. Handle the error or let it propagate.", Vice: "swallowed-error"},
		{ID: "tests-are-the-oracle", When: "command", Match: `^(npm|pnpm|yarn)\s+(run\s+)?test\b`, Infuse: "Passing tests is not the goal; the goal is the behavior the user asked for. Do not edit tests to make them pass."},
		{ID: "no-force-push", When: "command", Match: `git\s+push\s+(-f|--force)\b`, Replace: strPtr(": force push removed by infuse"), Say: "force push is not done in this project.", Vice: "destructive-git"},
	},
	Disable: []string{},
}

func strPtr(s string) *string { return &s }
