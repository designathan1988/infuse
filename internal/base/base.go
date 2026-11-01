// Package base owns the project's chosen architecture base: the folders a piece of work may live in,
// where data and design tokens belong, and the file-size standard. It is data the user chooses or
// edits in .infuse/base.json; without a base the check does nothing.
package base

import (
	"encoding/json"
	"fmt"
	"infuse/internal/core"
	"infuse/internal/data"
	"os"
	"path/filepath"
	"strings"
)

type Base struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Folders      []string `json:"folders"` // where the project's own code may live, relative to the root
	DataDir      string   `json:"dataDir,omitempty"`
	Tokens       string   `json:"tokens,omitempty"`
	MaxFileLines int      `json:"maxFileLines,omitempty"`
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

// Save records a starter by name or a full custom base.
func Save(root string, starterName string, custom *Base) error {
	if starterName != "" {
		if _, ok := starter(starterName); !ok {
			return fmt.Errorf("unknown base %q (catalog: %s)", starterName, catalogNames())
		}
	}
	if err := os.MkdirAll(filepath.Dir(file(root)), 0o755); err != nil {
		return err
	}
	if starterName != "" {
		return core.WriteJSON(file(root), map[string]string{"base": starterName})
	}
	if custom == nil || len(custom.Folders) == 0 {
		return fmt.Errorf("a custom base needs at least one folder")
	}
	return core.WriteJSON(file(root), custom)
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
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "./")
	for _, f := range b.Folders {
		f = strings.Trim(strings.TrimPrefix(filepath.ToSlash(f), "./"), "/")
		if rel == f || strings.HasPrefix(rel, f+"/") {
			return true
		}
	}
	return false
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

// Summary is the base as the agent reads it in the prompt.
func (b *Base) Summary() string {
	if b == nil {
		return ""
	}
	var parts []string
	parts = append(parts, "base: "+b.Name)
	if len(b.Folders) > 0 {
		parts = append(parts, "code lives in: "+strings.Join(b.Folders, ", "))
	}
	if b.DataDir != "" {
		parts = append(parts, "data/tables live in: "+b.DataDir)
	}
	if b.Tokens != "" {
		parts = append(parts, "design tokens live in: "+b.Tokens)
	}
	if b.MaxFileLines > 0 {
		parts = append(parts, fmt.Sprintf("files stay under %d lines", b.MaxFileLines))
	}
	return strings.Join(parts, "; ")
}
