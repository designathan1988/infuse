package base

// The scaffold: a base as a working starter, not empty folders. Canonical model and its owner, declared
// states, layers, a registry, config in one place, versioned persistence, an observability hook, a
// smoke test and the default oracle — real code from the embedded templates. A file that already
// exists is never touched.

import (
	"encoding/json"
	"fmt"
	"infuse/internal/data"
	"infuse/internal/journal"
	"infuse/internal/oracle"
	"infuse/internal/projectmap"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Scaffold creates a starter base in root: folders, starter code, the recorded choice, the default
// oracle, the base's document, the map pre-filled from all of it and the per-folder context files.
func Scaffold(root, name string) (Base, error) {
	b, ok := starter(name)
	if !ok {
		return Base{}, fmt.Errorf("unknown base %q (catalog: %s)", name, catalogNames())
	}
	if cur := Load(root); cur != nil && !strings.EqualFold(cur.Name, b.Name) {
		return b, fmt.Errorf("this project already uses base %q; .infuse/base.json is left as it is", cur.Name)
	}
	journal.InfuseDir(root)
	if err := makeFolders(root, b); err != nil {
		return b, err
	}
	if err := Save(root, b.Name, nil); err != nil {
		return b, err
	}
	if err := writeTemplates(root, b); err != nil {
		return b, err
	}
	if err := writeStarters(root, b); err != nil {
		return b, err
	}
	if err := writeDefaultOracle(root, b); err != nil {
		return b, err
	}
	if err := writeBaseDocument(root, b); err != nil {
		return b, err
	}
	projectmap.WriteMap(root, projectmap.BuildMap(root))
	return b, projectmap.SetContextFiles(root, true)
}

// makeFolders creates every declared folder and the data folder, which a base may declare only in its
// layout.
func makeFolders(root string, b Base) error {
	dirs := append([]string{}, b.Folders...)
	for _, f := range b.Layout {
		dirs = append(dirs, f.Path)
	}
	if b.DataDir != "" {
		dirs = append(dirs, b.DataDir)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			return err
		}
	}
	return nil
}

var (
	reNotModule  = regexp.MustCompile(`[^a-z0-9._-]+`)
	reNotPackage = regexp.MustCompile(`[^a-z0-9]+`)
)

// projectNames derives the module and package names from the project's folder.
func projectNames(root string) (module, pkg string) {
	abs, err := filepath.Abs(root)
	if err != nil {
		abs = root
	}
	low := strings.ToLower(filepath.Base(abs))
	module = strings.Trim(reNotModule.ReplaceAllString(low, "-"), "-._")
	pkg = reNotPackage.ReplaceAllString(low, "")
	if pkg == "" || pkg[0] >= '0' && pkg[0] <= '9' {
		pkg = "app" + pkg
	}
	if module == "" {
		module = pkg
	}
	return module, pkg
}

func writeTemplates(root string, b Base) error {
	module, pkg := projectNames(root)
	for _, t := range b.Templates {
		fill := strings.NewReplacer("{{module}}", module, "{{name}}", pkg, "{{base}}", b.Name, "{{package}}", templatePackage(t, pkg))
		to := filepath.Join(root, filepath.FromSlash(fill.Replace(t.To)))
		if err := writeNew(to, fill.Replace(string(data.Template(t.From)))); err != nil {
			return err
		}
	}
	return nil
}

// writeStarters writes each base folder's starter file (its README, saying what the folder is for).
func writeStarters(root string, b Base) error {
	for rel, content := range b.Starters {
		if err := writeNew(filepath.Join(root, filepath.FromSlash(rel)), content); err != nil {
			return err
		}
	}
	return nil
}

// templatePackage is the Go package a shared template lands in.
func templatePackage(t Template, projectPackage string) string {
	if t.Package != "" {
		return t.Package
	}
	return projectPackage
}

// writeNew writes a file that does not exist yet; an existing file is the user's.
func writeNew(file, text string) error {
	if _, err := os.Stat(file); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	return os.WriteFile(file, []byte(text), 0o644)
}

// writeDefaultOracle stores the base's default criteria and anchors them, unless the project has its own.
func writeDefaultOracle(root string, b Base) error {
	if len(b.Oracle) == 0 {
		return nil
	}
	if _, err := os.Stat(oracle.SpecFile(root)); err == nil {
		return nil
	}
	var spec oracle.OracleSpec
	if err := json.Unmarshal(b.Oracle, &spec); err != nil {
		return err
	}
	return oracle.SaveSpec(root, spec)
}

func writeBaseDocument(root string, b Base) error {
	doc := "# Architecture base: " + b.Name + "\n\n" + b.Contract()
	return writeNew(filepath.Join(root, "docs", "architecture", "base.md"), doc)
}
