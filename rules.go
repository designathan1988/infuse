package main

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
}

type RulesFile struct {
	Rules   []Rule   `json:"rules"`
	Disable []string `json:"disable"`
}

type RuleSet struct {
	Rules    []Rule
	Disabled map[string]bool
	Sources  []string
}

func globalRulesFile() string             { return filepath.Join(InfuseHome(), "rules.json") }
func projectRulesFile(root string) string { return filepath.Join(root, infuseDirName, "rules.json") }

func LoadRules(root string) *RuleSet {
	set := &RuleSet{Disabled: map[string]bool{}}
	for _, file := range []string{globalRulesFile(), projectRulesFile(root)} {
		if !exists(file) {
			continue
		}
		var rf RulesFile
		if err := readJSON(file, &rf); err != nil {
			set.Rules = append(set.Rules, Rule{When: "always", Say: fmt.Sprintf("rules file %s is not valid JSON: %v", file, err)})
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

func (r Rule) expand(root string) []string {
	var out []string
	if r.Say != "" {
		out = append(out, "[infuse] "+r.Say)
	}
	if r.Infuse != "" {
		out = append(out, r.Infuse)
	}
	if r.InfuseFile != "" {
		p := r.InfuseFile
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		if b, err := os.ReadFile(p); err == nil {
			out = append(out, strings.TrimRight(string(b), " \t\r\n"))
		} else {
			out = append(out, fmt.Sprintf("[infuse] rule %s: file not found: %s", r.ID, r.InfuseFile))
		}
	}
	return out
}

// ApplyCommandRules may rewrite the command; returns the trailer lines rules produce.
func ApplyCommandRules(root string, set *RuleSet, command string) (string, []string) {
	var lines []string
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
			Lesson(root, r.Vice, fmt.Sprintf("rule %s matched command: %s", r.ID, truncate(command, 120)))
		}
		lines = append(lines, r.expand(root)...)
	}
	return cmd, lines
}

// ApplyWriteRules checks the files written during a command (absolute paths).
func ApplyWriteRules(root string, set *RuleSet, files []string) []string {
	var lines []string
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
					lines = append(lines, fmt.Sprintf("[infuse] rule %s rewrote %s", r.ID, file))
				}
			}
			if r.Vice != "" {
				Lesson(root, r.Vice, fmt.Sprintf("rule %s matched in %s", r.ID, file))
			}
			for _, l := range r.expand(root) {
				lines = append(lines, strings.Replace(l, "[infuse] ", "[infuse] "+file+": ", 1))
			}
		}
	}
	return lines
}

// ApplyListRules: rules for a directory the agent listed (posix path relative to root).
func ApplyListRules(root string, set *RuleSet, dir string) []string {
	var lines []string
	for _, r := range set.Rules {
		if r.When != "list" {
			continue
		}
		if re := r.regex(); re != nil && !re.MatchString(dir) {
			continue
		}
		lines = append(lines, r.expand(root)...)
	}
	return lines
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

var exampleRules = RulesFile{
	Rules: []Rule{
		{ID: "no-swallowed-errors", When: "write", Match: `catch\s*(\([^)]*\))?\s*\{\s*\}`, Say: "empty catch block: the error is being swallowed. Handle the error or let it propagate.", Vice: "swallowed-error"},
		{ID: "tests-are-the-oracle", When: "command", Match: `^(npm|pnpm|yarn)\s+(run\s+)?test\b`, Infuse: "Passing tests is not the goal; the goal is the behavior the user asked for. Do not edit tests to make them pass."},
		{ID: "no-force-push", When: "command", Match: `git\s+push\s+(-f|--force)\b`, Replace: strPtr(": force push removed by infuse"), Say: "force push is not done in this project.", Vice: "destructive-git"},
	},
	Disable: []string{},
}

func strPtr(s string) *string { return &s }
