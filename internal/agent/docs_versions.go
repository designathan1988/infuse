package agent

// Dependency versions for doc suggestions: the pin should be the documentation of the version the
// project actually uses. Vendors that publish a versioned documentation URL are verified against the
// network before they enter the table; every other dependency keeps its stable URL with the version
// in the suggestion reason.

import (
	"fmt"
	"infuse/internal/knowledge"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// installedOr is the version in use, or the manifest's when the dependency is not installed here.
func installedOr(list []knowledge.Installed, dep, manifest string) string {
	for _, d := range list {
		if strings.EqualFold(d.Name, dep) {
			return d.Version
		}
	}
	return manifest
}

// maxExactPages bounds the suggestions a large lockfile adds, per ecosystem.
const maxExactPages = 12

// exactVersionPages suggests, for dependencies with no curated documentation, the registry's page of
// exactly the installed version: the URL carries the version, so it reads what will really run.
func exactVersionPages(list []knowledge.Installed, pinned, seen map[string]bool) []docSuggestion {
	var out []docSuggestion
	perEcosystem := map[string]int{}
	for _, d := range list {
		url := d.DocURL()
		_, curated := docsForDependency[d.Name]
		if url == "" || curated || pinned[url] || seen[url] || perEcosystem[d.Ecosystem] >= maxExactPages {
			continue
		}
		seen[url] = true
		perEcosystem[d.Ecosystem]++
		out = append(out, docSuggestion{Name: d.Name, URL: url, Why: M("docs.whyDependency", "package", d.Name, "version", d.Version).Render(uiLanguage())})
	}
	return out
}

// dependency → URL template and the version shape the vendor expects in it.
var versionedDocs = map[string][2]string{
	"django":  {"https://docs.djangoproject.com/en/%s/", "major.minor"},
	"flask":   {"https://flask.palletsprojects.com/en/%s.x/", "major.minor"},
	"pytest":  {"https://docs.pytest.org/en/%s.x/", "major.minor"},
	"express": {"https://expressjs.com/en/%sx/api.html", "major"},
}

// docURL returns the vendor's versioned documentation URL, or "" when the vendor has none or the
// installed version is unknown.
func docURL(dep, version string) string {
	t, ok := versionedDocs[dep]
	if !ok || strings.TrimSpace(version) == "" {
		return ""
	}
	shape := docVersionShape(version, t[1])
	if shape == "" {
		return ""
	}
	return fmt.Sprintf(t[0], shape)
}

// docVersionShape cuts an installed version down to the shape the vendor's URL expects.
func docVersionShape(version, shape string) string {
	parts := strings.Split(numericVersion(version), ".")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	minor := "0"
	if len(parts) > 1 && parts[1] != "" {
		minor = parts[1]
	}
	switch shape {
	case "major":
		return parts[0]
	case "major.minor":
		return parts[0] + "." + minor
	case "major.minor.x":
		return parts[0] + "." + minor + ".x"
	}
	return ""
}

var (
	reNumericVersion = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)
	reNpmSpec        = regexp.MustCompile(`\d+(?:\.\d+)*(?:-[0-9A-Za-z.]+)?`)
	rePyRequirement  = regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)(?:\[[^\]]*\])?\s*([^;#]*)`)
	rePyQuoted       = regexp.MustCompile(`"([^"\n]+)"|'([^'\n]+)'`)
	rePep621Deps     = regexp.MustCompile(`(?ms)^\s*dependencies\s*=\s*\[(.*?)\]`)
	rePoetryEntry    = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_.-]+)\s*=\s*(?:"([^"]+)"|\{[^}]*?version\s*=\s*"([^"]+)"[^}]*\})`)
)

func numericVersion(s string) string {
	if m := reNumericVersion.FindStringSubmatch(strings.TrimSpace(s)); m != nil {
		return m[1]
	}
	return ""
}

// dependencyVersions is name → the constraint the manifest declares, "" when the name is present
// without one.
type dependencyVersions map[string]string

// npmDependencyVersions reads the dependency ranges of package.json.
func npmDependencyVersions(root string) dependencyVersions {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if readJSON(filepath.Join(root, "package.json"), &pkg) != nil {
		return dependencyVersions{}
	}
	out := dependencyVersions{}
	for name, spec := range pkg.Dependencies {
		out[normalizeNpmName(name)] = firstNpmSpec(spec)
	}
	for name, spec := range pkg.DevDependencies {
		if _, ok := out[name]; !ok {
			out[normalizeNpmName(name)] = firstNpmSpec(spec)
		}
	}
	return out
}

func normalizeNpmName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// firstNpmSpec keeps the first concrete version of a range ("^4.19.2 || ^5" → "4.19.2"); ranges of
// disjoint majors keep the lower one, which is what the doc site of the older major documents.
func firstNpmSpec(spec string) string {
	return strings.TrimSpace(reNpmSpec.FindString(strings.TrimSpace(spec)))
}

// pythonDependencyVersions reads the pinned versions of the dependencies that have doc suggestions,
// from requirements.txt (PEP 508) and pyproject.toml (PEP 621 and Poetry). An exact pin wins over a
// compatible release, which wins over a lower bound; a dependency declared with no constraint is
// present with an empty spec.
func pythonDependencyVersions(root string) dependencyVersions {
	wanted := map[string]bool{"django": true, "flask": true, "fastapi": true, "pytest": true}
	specs := dependencyVersions{}
	present := dependencyVersions{}
	add := func(name, spec string) {
		name = normalizePyName(name)
		if !wanted[name] {
			return
		}
		present[name] = ""
		if s := pyConstraint(spec); s != "" {
			specs[name] = appendPySpec(specs[name], s)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "requirements.txt")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
				continue
			}
			if m := rePyRequirement.FindStringSubmatch(line); m != nil {
				add(m[1], m[2])
			}
		}
		for _, name := range unversionedNames(string(b), wanted, present) {
			add(name, "")
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
		text := string(b)
		for _, spec := range pep621Specs(text) {
			add(pep508Name(spec), pep508Spec(spec))
		}
		for _, m := range rePoetryEntry.FindAllStringSubmatch(poetryDependencies(text), -1) {
			v := m[2]
			if v == "" {
				v = m[3]
			}
			add(m[1], v)
		}
		for _, name := range unversionedNames(text, wanted, present) {
			add(name, "")
		}
	}
	out := dependencyVersions{}
	for name := range present {
		out[name] = choosePySpec(specs[name])
	}
	return out
}

func normalizePyName(name string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "_", "-"))
}

// unversionedNames lists the wanted dependencies the version parsers could not see in text, so they
// are still suggested with their stable URL.
func unversionedNames(text string, wanted map[string]bool, present dependencyVersions) []string {
	low := strings.ToLower(text)
	var out []string
	for name := range wanted {
		if _, ok := present[name]; !ok && strings.Contains(low, name) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// pep621Specs returns the quoted strings of the PEP 621 `dependencies` array.
func pep621Specs(text string) []string {
	m := rePep621Deps.FindStringSubmatch(text)
	if m == nil {
		return nil
	}
	var out []string
	for _, q := range rePyQuoted.FindAllStringSubmatch(m[1], -1) {
		if q[1] != "" {
			out = append(out, q[1])
		} else if q[2] != "" {
			out = append(out, q[2])
		}
	}
	return out
}

// poetryDependencies returns the body of the [tool.poetry.dependencies] section.
func poetryDependencies(text string) string {
	start := strings.Index(text, "[tool.poetry.dependencies]")
	if start < 0 {
		return ""
	}
	rest := text[start+len("[tool.poetry.dependencies]"):]
	if end := strings.Index(rest, "\n["); end >= 0 {
		rest = rest[:end]
	}
	return rest
}

func pep508Name(spec string) string {
	if m := rePyRequirement.FindStringSubmatch(strings.TrimSpace(spec)); m != nil {
		return m[1]
	}
	return strings.TrimSpace(spec)
}

func pep508Spec(spec string) string {
	if m := rePyRequirement.FindStringSubmatch(strings.TrimSpace(spec)); m != nil {
		return m[2]
	}
	return ""
}

// pyConstraint drops environment markers ("; python_version >= '3.9'") and extras from a spec.
func pyConstraint(spec string) string {
	s := strings.TrimSpace(strings.SplitN(spec, ";", 2)[0])
	return strings.TrimSpace(s)
}

// appendPySpec collects the constraints of one dependency across manifests, comma-joined.
func appendPySpec(current, spec string) string {
	if current == "" {
		return spec
	}
	return current + "," + spec
}

// choosePySpec prefers an exact pin ("=="), then a compatible release ("~="), then the strongest
// lower bound; a bare Poetry version acts as an exact pin.
func choosePySpec(specs string) string {
	best, bestRank := "", 0
	for _, part := range strings.Split(specs, ",") {
		part = strings.TrimSpace(strings.Trim(part, `"'`))
		op, rest := "", part
		for _, p := range []string{"===", "==", "~=", ">=", "^", ">"} {
			if after, ok := strings.CutPrefix(part, p); ok {
				op, rest = p, strings.TrimSpace(after)
				break
			}
		}
		v := numericVersion(rest)
		if v == "" {
			continue
		}
		if best == "" || specRank(op) > bestRank {
			best, bestRank = v, specRank(op)
		}
	}
	return best
}

// specRank orders constraints: an exact pin (a bare Poetry version included) beats a compatible
// release, which beats a lower bound.
func specRank(op string) int {
	switch op {
	case "==", "===", "":
		return 3
	case "~=":
		return 2
	default:
		return 1
	}
}

// sortedDependencies gives deterministic suggestion order.
func sortedDependencies(v dependencyVersions) []string {
	names := make([]string, 0, len(v))
	for name := range v {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
