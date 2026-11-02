// Package base owns the project's chosen architecture base: the folders a piece of work may live in,
// where data and design tokens belong, the layers and their allowed dependencies, and the file-size
// standard. It is data the user chooses or edits in .infuse/base.json; without a base the check does
// nothing.
package base

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/data"
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"strings"
)

// FolderRole is one declared folder of a base and the responsibility it carries (projectmap.FolderRole,
// the same shape the map publishes).
type FolderRole = projectmap.FolderRole

// Layer is one layer and the layers it may depend on.
type Layer = projectmap.Layer

// Datum names one piece of canonical data: where it is declared, who alone writes it, its states.
type Datum struct {
	Datum      string   `json:"datum"`
	Owner      string   `json:"owner"`
	DeclaredIn string   `json:"declaredIn,omitempty"`
	States     []string `json:"states,omitempty"`
}

// Registry is an extension point: new behavior is a new row, not a new conditional.
type Registry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// Template is one starter file: from names an embedded template, to where it lands in the project.
type Template struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Package string `json:"package,omitempty"`
}

type Base struct {
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Language      string            `json:"language,omitempty"`
	Folders       []string          `json:"folders"`          // where the project's own code may live, relative to the root
	Layout        []FolderRole      `json:"layout,omitempty"` // what each declared folder is for (the intended architecture)
	Layers        []Layer           `json:"layers,omitempty"` // layers and their allowed dependencies
	Public        []string          `json:"public,omitempty"` // folders whose exports are the product's surface
	Model         []Datum           `json:"model,omitempty"`
	Registries    []Registry        `json:"registries,omitempty"`
	Config        string            `json:"config,omitempty"`        // the one place for settings and tables
	Persistence   string            `json:"persistence,omitempty"`   // the versioned persistence helper
	Observability string            `json:"observability,omitempty"` // the observability hook
	Surface       string            `json:"surface,omitempty"`       // what the oracle can observe
	DataDir       string            `json:"dataDir,omitempty"`
	Tokens        string            `json:"tokens,omitempty"`
	MaxFileLines  int               `json:"maxFileLines,omitempty"`
	Templates     []Template        `json:"templates,omitempty"`
	Oracle        json.RawMessage   `json:"oracle,omitempty"`   // the default .infuse/oracle/checks.json
	Roles         map[string]string `json:"roles,omitempty"`    // folder → its responsibility (module meaning)
	Starters      map[string]string `json:"starters,omitempty"` // relative path → starter file content (never overwrites)
}

func file(root string) string { return filepath.Join(root, core.InfuseDirName, "base.json") }

// Catalog lists the starter bases shipped with infuse.
func Catalog() []Base {
	var list []Base
	_ = json.Unmarshal(data.Read("bases.json"), &list)
	return list
}

func starter(name string) (Base, bool) {
	for _, b := range Catalog() {
		if strings.EqualFold(b.Name, name) {
			return b, true
		}
	}
	return Base{}, false
}

// Load returns the project's base, or nil when the user has not chosen one. The file is either
// {"base":"web"} (a starter) or a full custom Base.
func Load(root string) *Base {
	b, err := os.ReadFile(file(root))
	if err != nil {
		return nil
	}
	var choose struct {
		Base string `json:"base"`
	}
	if json.Unmarshal(b, &choose) == nil && choose.Base != "" {
		if s, ok := starter(choose.Base); ok {
			return &s
		}
		return nil
	}
	var custom Base
	if json.Unmarshal(b, &custom) != nil || custom.Name == "" && len(custom.Folders) == 0 {
		return nil
	}
	if custom.Name == "" {
		custom.Name = "custom"
	}
	return &custom
}

// Save records a starter by name or a full custom base, and records the base's declared structure as
// the project's intended architecture — choosing a base already pre-fills the map.
func Save(root string, starterName string, custom *Base) error {
	chosen, err := resolveForSave(starterName, custom)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file(root)), 0o755); err != nil {
		return err
	}
	if starterName != "" {
		if err := core.WriteJSON(file(root), map[string]string{"base": starterName}); err != nil {
			return err
		}
	} else if err := core.WriteJSON(file(root), custom); err != nil {
		return err
	}
	return projectmap.WriteContract(root, chosen.contract())
}

// contract is what the map needs to know of the base.
func (b Base) contract() projectmap.Contract {
	return projectmap.Contract{Layout: b.Layout, Folders: b.Folders, Layers: b.Layers, Public: b.Public, MaxFileLines: b.MaxFileLines}
}

// resolveForSave checks the arguments and returns the base they stand for, for the intended map.
func resolveForSave(starterName string, custom *Base) (Base, error) {
	if starterName != "" {
		b, ok := starter(starterName)
		if !ok {
			return Base{}, fmt.Errorf("unknown base %q (catalog: %s)", starterName, catalogNames())
		}
		return b, nil
	}
	if custom == nil || len(custom.Folders) == 0 {
		return Base{}, fmt.Errorf("a custom base needs at least one folder")
	}
	out := *custom
	if out.Name == "" {
		out.Name = "custom"
	}
	if len(out.Layout) == 0 {
		for _, f := range out.Folders {
			out.Layout = append(out.Layout, FolderRole{Path: f})
		}
	}
	return out, nil
}

func catalogNames() string {
	var names []string
	for _, b := range Catalog() {
		names = append(names, b.Name)
	}
	return strings.Join(names, ", ")
}

// Declares reports whether rel is inside one of the base's folders.
func (b *Base) Declares(rel string) bool {
	return projectmap.Within(strings.TrimPrefix(filepath.ToSlash(rel), "./"), b.Folders)
}

// CheckWrite refuses a code file written outside the chosen base. Root-level files (manifests,
// readmes and configuration) are always allowed; the check is about where code lives.
func CheckWrite(root, rel string, isCode bool) string {
	b := Load(root)
	if b == nil || !isCode || !strings.Contains(filepath.ToSlash(rel), "/") {
		return ""
	}
	if b.Declares(rel) {
		return ""
	}
	return b.Name
}

// CheckLayers looks at what content imports: the rule broken and the import that broke it, or empty
// strings when the base has no objection.
func CheckLayers(root, rel, content string) (string, string) {
	b := Load(root)
	if b == nil || len(b.Layers) == 0 {
		return "", ""
	}
	return b.contract().ImportViolation(root, strings.TrimPrefix(filepath.ToSlash(rel), "./"), content)
}

// Summary is the base in one line.
func (b *Base) Summary() string {
	if b == nil {
		return ""
	}
	var parts []string
	parts = append(parts, "base: "+b.Name)
	if len(b.Folders) > 0 {
		parts = append(parts, "code lives in: "+strings.Join(b.Folders, ", "))
	}
	for _, f := range b.Layout {
		parts = append(parts, f.Path+" — "+f.Role)
	}
	if b.DataDir != "" {
		parts = append(parts, "data/tables live in: "+b.DataDir)
	}
	if b.Tokens != "" {
		parts = append(parts, "design tokens live in: "+b.Tokens)
	}
	if len(b.Roles) > 0 {
		var roles []string
		for _, f := range b.Folders {
			if role := b.Roles[f]; role != "" {
				roles = append(roles, f+" ("+role+")")
			}
		}
		if len(roles) > 0 {
			parts = append(parts, "responsibilities: "+strings.Join(roles, ", "))
		}
	}
	if b.MaxFileLines > 0 {
		parts = append(parts, fmt.Sprintf("files stay under %d lines", b.MaxFileLines))
	}
	return strings.Join(parts, "; ")
}

// Contract is the base's whole contract as the agent reads it: every element a base defines (SPEC §4.2).
func (b *Base) Contract() string {
	if b == nil {
		return ""
	}
	var s strings.Builder
	fmt.Fprintf(&s, "Base %s: %s\n", b.Name, b.Description)
	if len(b.Layout) > 0 {
		s.WriteString("Folders and what each one is for:\n")
		for _, f := range b.Layout {
			fmt.Fprintf(&s, "- %s: %s\n", f.Path, f.Role)
		}
	} else if len(b.Folders) > 0 {
		s.WriteString("Code lives in: " + strings.Join(b.Folders, ", ") + "\n")
	}
	writeLayers(&s, b.Layers)
	writeModel(&s, b.Model)
	for _, r := range b.Registries {
		fmt.Fprintf(&s, "Extension point %s in %s: %s.\n", r.Name, r.Path, r.Purpose)
	}
	writeHome(&s, "Settings and tables", b.Config)
	writeHome(&s, "Data tables", b.DataDir)
	writeHome(&s, "Design tokens (the only source of colors and sizes)", b.Tokens)
	writeHome(&s, "Versioned persistence goes through", b.Persistence)
	writeHome(&s, "Observability goes through", b.Observability)
	writeHome(&s, "What the oracle observes", b.Surface)
	if b.MaxFileLines > 0 {
		fmt.Fprintf(&s, "No code file past %d lines.\n", b.MaxFileLines)
	}
	return s.String()
}

func writeHome(s *strings.Builder, label, value string) {
	if value != "" {
		fmt.Fprintf(s, "%s: %s.\n", label, value)
	}
}

func writeLayers(s *strings.Builder, layers []Layer) {
	if len(layers) == 0 {
		return
	}
	s.WriteString("Layers (an import outside this list is refused):\n")
	for _, l := range layers {
		deps := "nothing else in the project"
		if len(l.MayImport) > 0 {
			deps = strings.Join(l.MayImport, ", ")
		}
		fmt.Fprintf(s, "- %s may import: %s\n", l.Path, deps)
	}
}

func writeModel(s *strings.Builder, model []Datum) {
	for _, d := range model {
		fmt.Fprintf(s, "Datum %s: owner %s, the only writer", d.Datum, d.Owner)
		if d.DeclaredIn != "" && d.DeclaredIn != d.Owner {
			fmt.Fprintf(s, "; declared in %s", d.DeclaredIn)
		}
		if len(d.States) > 0 {
			fmt.Fprintf(s, "; declared states: %s", strings.Join(d.States, ", "))
		}
		s.WriteString(".\n")
	}
}
