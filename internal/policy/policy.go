// Package policy owns the user's behaviour policy: how the agent is allowed to work, what it must do
// before acting, the architecture and convention standards, and the definition of done. It is one
// versioned document per scope (global and project) that the Rules surface edits; the filter and the
// anti-fraud core themselves are not configurable.
package policy

import (
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/store"
	"os"
	"path/filepath"
)

type Policy struct {
	store.Versioned
	Permissions  Permissions  `json:"permissions"`
	Prohibitions Prohibitions `json:"prohibitions"`
	Method       Method       `json:"method"`
	Architecture Architecture `json:"architecture"`
	Conventions  Conventions  `json:"conventions"`
	Done         DonePolicy   `json:"done"`
}

type Permissions struct {
	Mode           string            `json:"mode,omitempty"` // "" = keep the settings value
	Tools          map[string]string `json:"tools,omitempty"`
	ProtectedPaths []string          `json:"protectedPaths,omitempty"`
}

type Prohibitions struct {
	Commands []string `json:"commands,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Tools    []string `json:"tools,omitempty"`
	Actions  []string `json:"actions,omitempty"` // commit | push | install
}

type Method struct {
	Plan            *bool `json:"plan,omitempty"`
	Diagnosis       *bool `json:"diagnosis,omitempty"`
	DocsBeforeUse   *bool `json:"docsBeforeUse,omitempty"`
	ReadingCoverage *bool `json:"readingCoverage,omitempty"`
	AskBudget       *int  `json:"askBudget,omitempty"`
}

type Architecture struct {
	MaxFileLines *int  `json:"maxFileLines,omitempty"`
	Enforce      *bool `json:"enforce,omitempty"`
}

type Conventions struct {
	Enforce *bool `json:"enforce,omitempty"`
}

type DonePolicy struct {
	Review          *bool `json:"review,omitempty"`
	RequireContract *bool `json:"requireContract,omitempty"`
}

func boolp(v bool) *bool { return &v }
func intp(v int) *int    { return &v }

// Default is what the Rules surface shows before the user changes anything: every protection on.
func Default() Policy {
	return Policy{
		Permissions: Permissions{Tools: map[string]string{}},
		Method: Method{
			Plan: boolp(true), Diagnosis: boolp(true), DocsBeforeUse: boolp(true),
			ReadingCoverage: boolp(true), AskBudget: intp(6),
		},
		Architecture: Architecture{MaxFileLines: intp(400), Enforce: boolp(true)},
		Conventions:  Conventions{Enforce: boolp(true)},
		Done:         DonePolicy{Review: boolp(true), RequireContract: boolp(true)},
	}
}

func globalFile() string { return filepath.Join(core.InfuseHome(), "policy.json") }
func projectFile(root string) string {
	return filepath.Join(root, core.InfuseDirName, "policy.json")
}

// LoadScopes reads the two files, returning nil for a scope that does not exist. A corrupt file is an
// error: the policy must never silently fall back to something weaker.
func LoadScopes(root string) (global, project *Policy, err error) {
	global, err = readFile(globalFile())
	if err != nil {
		return nil, nil, fmt.Errorf("global policy: %w", err)
	}
	project, err = readFile(projectFile(root))
	if err != nil {
		return nil, nil, fmt.Errorf("project policy: %w", err)
	}
	return global, project, nil
}

func readFile(path string) (*Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	_, _ = p.Upgrade(nil)
	if err := Validate(p); err != nil {
		return nil, err
	}
	return &p, nil
}

// Effective overlays the scopes on the defaults, in order: defaults, global, project.
func Effective(root string) (Policy, error) {
	out := Default()
	global, project, err := LoadScopes(root)
	if err != nil {
		return out, err
	}
	overlay(&out, global)
	overlay(&out, project)
	return out, nil
}

func overlay(out *Policy, p *Policy) {
	if p == nil {
		return
	}
	if p.Permissions.Mode != "" {
		out.Permissions.Mode = p.Permissions.Mode
	}
	for k, v := range p.Permissions.Tools {
		out.Permissions.Tools[k] = v
	}
	out.Permissions.ProtectedPaths = append(out.Permissions.ProtectedPaths, p.Permissions.ProtectedPaths...)
	out.Prohibitions.Commands = append(out.Prohibitions.Commands, p.Prohibitions.Commands...)
	out.Prohibitions.Paths = append(out.Prohibitions.Paths, p.Prohibitions.Paths...)
	out.Prohibitions.Tools = append(out.Prohibitions.Tools, p.Prohibitions.Tools...)
	out.Prohibitions.Actions = append(out.Prohibitions.Actions, p.Prohibitions.Actions...)
	setBool(&out.Method.Plan, p.Method.Plan)
	setBool(&out.Method.Diagnosis, p.Method.Diagnosis)
	setBool(&out.Method.DocsBeforeUse, p.Method.DocsBeforeUse)
	setBool(&out.Method.ReadingCoverage, p.Method.ReadingCoverage)
	setInt(&out.Method.AskBudget, p.Method.AskBudget)
	setInt(&out.Architecture.MaxFileLines, p.Architecture.MaxFileLines)
	setBool(&out.Architecture.Enforce, p.Architecture.Enforce)
	setBool(&out.Conventions.Enforce, p.Conventions.Enforce)
	setBool(&out.Done.Review, p.Done.Review)
	setBool(&out.Done.RequireContract, p.Done.RequireContract)
}

func setBool(dst **bool, src *bool) {
	if src != nil {
		*dst = src
	}
}

func setInt(dst **int, src *int) {
	if src != nil {
		*dst = src
	}
}

// ApplySettings overlays the explicit policy values on a session's settings. Scopes are the raw files:
// a value the user never set stays whatever the settings say.
func ApplySettings(s *config.Settings, scopes ...*Policy) {
	for _, p := range scopes {
		if p == nil {
			continue
		}
		if p.Permissions.Mode != "" {
			s.Mode = p.Permissions.Mode
		}
		if s.Permissions == nil {
			s.Permissions = map[string]string{}
		}
		for k, v := range p.Permissions.Tools {
			s.Permissions[k] = v
		}
		s.ProtectedPaths = dedupe(append(s.ProtectedPaths, p.Permissions.ProtectedPaths...))
		if p.Method.Plan != nil {
			s.RequirePlan = p.Method.Plan
		}
		if p.Method.Diagnosis != nil {
			s.RequireDiagnosis = p.Method.Diagnosis
		}
		if p.Method.DocsBeforeUse != nil {
			s.RequireDocs = p.Method.DocsBeforeUse
		}
		if p.Method.ReadingCoverage != nil {
			s.RequireReading = p.Method.ReadingCoverage
		}
		if p.Method.AskBudget != nil {
			s.AskBudget = *p.Method.AskBudget
		}
		if p.Architecture.MaxFileLines != nil {
			s.MaxFileLines = *p.Architecture.MaxFileLines
		}
		if p.Architecture.Enforce != nil {
			s.EnforceArchitecture = p.Architecture.Enforce
		}
		if p.Conventions.Enforce != nil {
			s.EnforceConventions = p.Conventions.Enforce
		}
		if p.Done.Review != nil {
			s.RequireReview = p.Done.Review
		}
		if p.Done.RequireContract != nil {
			s.RequireContract = p.Done.RequireContract
		}
	}
}

func dedupe(list []string) []string {
	seen := map[string]bool{}
	out := list[:0]
	for _, v := range list {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// ProhibitionsFrom unions the prohibitions of the scopes.
func ProhibitionsFrom(scopes ...*Policy) Prohibitions {
	var out Prohibitions
	for _, p := range scopes {
		if p == nil {
			continue
		}
		out.Commands = append(out.Commands, p.Prohibitions.Commands...)
		out.Paths = append(out.Paths, p.Prohibitions.Paths...)
		out.Tools = append(out.Tools, p.Prohibitions.Tools...)
		out.Actions = append(out.Actions, p.Prohibitions.Actions...)
	}
	out.Commands = dedupe(out.Commands)
	out.Paths = dedupe(out.Paths)
	out.Tools = dedupe(out.Tools)
	out.Actions = dedupe(out.Actions)
	return out
}

// Save writes one scope. The caller chooses global or project; a project policy applies to that folder.
func Save(root string, global bool, p Policy) error {
	if err := Validate(p); err != nil {
		return err
	}
	p.SchemaVersion = store.Version
	path := projectFile(root)
	if global {
		path = globalFile()
	}
	return core.WriteJSON(path, p)
}

// Validate refuses values that would silently weaken or break the policy engine.
func Validate(p Policy) error {
	switch p.Permissions.Mode {
	case "", "default", "acceptEdits", "plan", "bypass":
	default:
		return fmt.Errorf("unknown permission mode %q", p.Permissions.Mode)
	}
	for tool, v := range p.Permissions.Tools {
		if v != "ask" && v != "allow" && v != "deny" {
			return fmt.Errorf("tool %q: %q is not ask, allow or deny", tool, v)
		}
	}
	if p.Method.AskBudget != nil && (*p.Method.AskBudget < 1 || *p.Method.AskBudget > 20) {
		return fmt.Errorf("askBudget must be between 1 and 20")
	}
	if p.Architecture.MaxFileLines != nil && *p.Architecture.MaxFileLines != -1 && (*p.Architecture.MaxFileLines < 20 || *p.Architecture.MaxFileLines > 5000) {
		return fmt.Errorf("maxFileLines must be -1 (off) or between 20 and 5000")
	}
	for _, a := range p.Prohibitions.Actions {
		if a != "commit" && a != "push" && a != "install" {
			return fmt.Errorf("unknown prohibited action %q", a)
		}
	}
	return nil
}

// File returns the path of a scope's document, for the UI.
func File(root string, global bool) string {
	if global {
		return globalFile()
	}
	return projectFile(root)
}
