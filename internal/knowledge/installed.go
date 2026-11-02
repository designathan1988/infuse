package knowledge

// The versions actually installed, not the ranges a manifest allows: node_modules/*/package.json (then
// the lockfile), go.sum with go.mod's selection, Cargo.lock, poetry.lock, and exact pins in
// requirements.txt. Perception reads what will really run.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Ecosystems a dependency can come from.
const (
	EcoNpm  = "npm"
	EcoGo   = "go"
	EcoRust = "rust"
	EcoPy   = "python"
)

// Installed is one dependency at the version in use.
type Installed struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
	Version   string `json:"version"`
}

// DocURL is the page of exactly this version, where the ecosystem's documentation host keeps one.
func (d Installed) DocURL() string {
	switch d.Ecosystem {
	case EcoNpm:
		return "https://www.npmjs.com/package/" + d.Name + "/v/" + d.Version
	case EcoGo:
		return "https://pkg.go.dev/" + d.Name + "@" + d.Version
	case EcoRust:
		return "https://docs.rs/" + d.Name + "/" + d.Version
	case EcoPy:
		return "https://pypi.org/project/" + d.Name + "/" + d.Version + "/"
	}
	return ""
}

// InstalledDependencies reads every source, sorted by ecosystem and name.
func InstalledDependencies(root string) []Installed {
	var out []Installed
	for _, read := range []func(string) []Installed{installedNpm, installedGo, installedCargo, installedPython} {
		out = append(out, read(root)...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Ecosystem != out[j].Ecosystem {
			return out[i].Ecosystem < out[j].Ecosystem
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// InstalledVersion finds one dependency's version in use ("" when it is not installed here).
func InstalledVersion(root, name string) string {
	for _, d := range InstalledDependencies(root) {
		if strings.EqualFold(d.Name, name) {
			return d.Version
		}
	}
	return ""
}

// installedNpm answers for the manifest's direct dependencies: the package.json inside node_modules
// is what runs, the lockfile stands in when nothing is installed.
func installedNpm(root string) []Installed {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	b, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil || json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	locked := npmLockVersions(root)
	var out []Installed
	seen := map[string]bool{}
	for _, list := range []map[string]string{pkg.Dependencies, pkg.DevDependencies} {
		for name := range list {
			if seen[name] {
				continue
			}
			seen[name] = true
			v := npmInstalledVersion(root, name)
			if v == "" {
				v = locked[name]
			}
			if v != "" {
				out = append(out, Installed{EcoNpm, name, v})
			}
		}
	}
	return out
}

func npmInstalledVersion(root, name string) string {
	var p struct {
		Version string `json:"version"`
	}
	b, err := os.ReadFile(filepath.Join(root, "node_modules", filepath.FromSlash(name), "package.json"))
	if err != nil || json.Unmarshal(b, &p) != nil {
		return ""
	}
	return p.Version
}

func npmLockVersions(root string) map[string]string {
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	out := map[string]string{}
	b, err := os.ReadFile(filepath.Join(root, "package-lock.json"))
	if err != nil || json.Unmarshal(b, &lock) != nil {
		return out
	}
	for k, v := range lock.Packages {
		if name, ok := strings.CutPrefix(k, "node_modules/"); ok && !strings.Contains(name, "/node_modules/") {
			out[name] = v.Version
		}
	}
	return out
}

var (
	reGoRequire = regexp.MustCompile(`(?m)^\s*(?:require\s+)?([a-zA-Z0-9.\-]+\.[a-z]+/[^\s]+)\s+(v[^\s]+)(\s*//\s*indirect)?`)
	reGoReplace = regexp.MustCompile(`(?m)^\s*(?:replace\s+)?[^\s]+(?:\s+v[^\s]+)?\s+=>\s+([^\s]+)\s+(v[^\s]+)`)
)

// installedGo reads the versions go.mod selects, keeping those go.sum confirms when a go.sum exists:
// indirect requirements are not what the project's code calls.
func installedGo(root string) []Installed {
	mod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil
	}
	sum, sumErr := os.ReadFile(filepath.Join(root, "go.sum"))
	replaced := map[string]string{}
	for _, m := range reGoReplace.FindAllStringSubmatch(string(mod), -1) {
		replaced[m[1]] = m[2]
	}
	var out []Installed
	for _, m := range reGoRequire.FindAllStringSubmatch(string(mod), -1) {
		if m[3] != "" {
			continue
		}
		version := m[2]
		if v := replaced[m[1]]; v != "" {
			version = v
		}
		if sumErr == nil && !strings.Contains(string(sum), m[1]+" "+version+" ") {
			continue
		}
		out = append(out, Installed{EcoGo, m[1], version})
	}
	return out
}

var reLockPackage = regexp.MustCompile(`(?m)^\[\[package\]\]\s*\nname\s*=\s*"([^"]+)"\s*\nversion\s*=\s*"([^"]+)"`)

// installedCargo reads Cargo.lock, skipping the workspace's own crates (no source line).
func installedCargo(root string) []Installed {
	b, err := os.ReadFile(filepath.Join(root, "Cargo.lock"))
	if err != nil {
		return nil
	}
	var out []Installed
	for _, block := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n\n") {
		if m := reLockPackage.FindStringSubmatch(block); m != nil && strings.Contains(block, "\nsource = ") {
			out = append(out, Installed{EcoRust, m[1], m[2]})
		}
	}
	return out
}

var reRequirementPin = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_.-]+)(?:\[[^\]]*\])?\s*===?\s*([0-9][^\s;#,]*)`)

// installedPython reads poetry.lock, or the exact pins of requirements.txt when there is no lockfile.
func installedPython(root string) []Installed {
	if b, err := os.ReadFile(filepath.Join(root, "poetry.lock")); err == nil {
		var out []Installed
		for _, m := range reLockPackage.FindAllStringSubmatch(strings.ReplaceAll(string(b), "\r\n", "\n"), -1) {
			out = append(out, Installed{EcoPy, strings.ToLower(m[1]), m[2]})
		}
		return out
	}
	b, err := os.ReadFile(filepath.Join(root, "requirements.txt"))
	if err != nil {
		return nil
	}
	var out []Installed
	for _, m := range reRequirementPin.FindAllStringSubmatch(string(b), -1) {
		out = append(out, Installed{EcoPy, strings.ToLower(m[1]), m[2]})
	}
	return out
}

// InstalledPrompt states the versions in use, with the page of each, for the system prompt and the
// grounds of an external agent.
func InstalledPrompt(root string, limit int) string {
	var b strings.Builder
	for i, d := range InstalledDependencies(root) {
		if i >= limit {
			fmt.Fprintf(&b, "- … and more; the lockfiles have them all\n")
			break
		}
		fmt.Fprintf(&b, "- %s %s: %s\n", d.Name, d.Version, d.DocURL())
	}
	return b.String()
}
