package projectmap

// What one source file exports and imports, per language, by regular expression: the map is a fast
// reading of the code, not a compiler.

import (
	"path"
	"regexp"
	"sort"
	"strings"
)

var langByExt = map[string]string{
	".ts": "ts", ".tsx": "ts", ".mts": "ts", ".cts": "ts",
	".js": "js", ".jsx": "js", ".mjs": "js", ".cjs": "js",
	".vue": "vue", ".svelte": "svelte",
	".py": "py", ".go": "go", ".rs": "rs", ".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".cs": "cs",
	".c": "c", ".h": "c", ".cpp": "cpp", ".cc": "cpp", ".cxx": "cpp", ".hpp": "cpp", ".hh": "cpp",
	".rb": "rb", ".php": "php", ".css": "css", ".scss": "css", ".html": "html",
	".json": "json", ".md": "md", ".yml": "yaml", ".yaml": "yaml", ".toml": "toml", ".sh": "sh",
}

// nonCodeLangs are mapped for size and location, not for structure.
var nonCodeLangs = map[string]bool{"css": true, "html": true, "json": true, "md": true, "yaml": true, "toml": true, "sh": true}

func langOf(name string) (string, bool) {
	l, ok := langByExt[strings.ToLower(path.Ext(name))]
	return l, ok
}

var (
	reExpJs1   = regexp.MustCompile(`(?m)^export\s+(?:default\s+)?(?:async\s+)?(?:function\*?|class|const|let|var|interface|type|enum|abstract\s+class)\s+([A-Za-z_$][\w$]*)`)
	reExpJs2   = regexp.MustCompile(`(?m)^export\s*\{([^}]*)\}`)
	reExpJsDef = regexp.MustCompile(`(?m)^export\s+default\b`)
	reExpCjs   = regexp.MustCompile(`(?m)^module\.exports\s*=\s*\{([^}]*)\}`)
	reJsAs     = regexp.MustCompile(`\s+as\s+`)
	reExpPy    = regexp.MustCompile(`(?m)^(?:def|class)\s+([A-Za-z_]\w*)`)
	reExpGo1   = regexp.MustCompile(`(?m)^func\s+(?:\([^)]*\)\s*)?([A-Z]\w*)`)
	reExpGo2   = regexp.MustCompile(`(?m)^type\s+([A-Z]\w*)`)
	reExpRs    = regexp.MustCompile(`(?m)^\s*pub\s+(?:async\s+)?(?:fn|struct|enum|trait|type|mod|const|static)\s+([A-Za-z_]\w*)`)
	reExpCs    = regexp.MustCompile(`(?m)^\s*(?:public|internal|open|data|sealed)\s+(?:\w+\s+)*?(?:class|interface|record|enum|struct|object)\s+([A-Za-z_]\w*)`)
	reExpJvm   = regexp.MustCompile(`(?m)^\s*(?:(?:public|protected|internal|abstract|final|open|sealed|data|enum|annotation|value|static)\s+)*(?:class|interface|object|enum|record|@interface)\s+([A-Za-z_]\w*)`)
	reExpKtFun = regexp.MustCompile(`(?m)^(?:(?:public|internal|suspend|inline)\s+)*fun\s+(?:<[^>]+>\s*)?([A-Za-z_]\w*)`)
	reExpCType = regexp.MustCompile(`(?m)^(?:typedef\s+)?(?:struct|class|enum|union)\s+([A-Za-z_]\w*)`)
	reExpCFunc = regexp.MustCompile(`(?m)^[A-Za-z_][\w:<>*&\t ]*?[\t *&]([A-Za-z_]\w*)\s*\([^;{}]*\)\s*(?:const\s*)?[;{]`)

	reImpJs1 = regexp.MustCompile(`(?m)^\s*(?:import|export)\s+(?:[^'"]*?\s+from\s+)?['"]([^'"]+)['"]`)
	reImpJs2 = regexp.MustCompile(`\brequire\(\s*['"]([^'"]+)['"]\s*\)`)
	reImpJs3 = regexp.MustCompile(`\bimport\(\s*['"]([^'"]+)['"]\s*\)`)
	reImpPy1 = regexp.MustCompile(`(?m)^\s*from\s+([\w.]+)\s+import`)
	reImpPy2 = regexp.MustCompile(`(?m)^\s*import\s+([\w.]+)`)
	reImpGo  = regexp.MustCompile(`(?m)^\s*(?:[\w.]+\s+)?"([^"]+)"\s*$`)
	reImpGo1 = regexp.MustCompile(`(?m)^\s*import\s+(?:[\w.]+\s+)?"([^"]+)"`)
	reImpRs1 = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?use\s+([\w:]+)`)
	reImpRs2 = regexp.MustCompile(`(?m)^\s*mod\s+(\w+)\s*;`)
	reImpCs  = regexp.MustCompile(`(?m)^\s*using\s+([\w.]+)\s*;`)
	reImpJv  = regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([\w.]+(?:\.\*)?)`)
	reImpC   = regexp.MustCompile(`(?m)^\s*#\s*include\s+"([^"]+)"`)

	reNamespace = regexp.MustCompile(`(?m)^\s*namespace\s+([\w.]+)`)
	rePackage   = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)`)
)

// cKeywords look like function names to the regular expression.
var cKeywords = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "return": true, "sizeof": true}

func addAll(set map[string]bool, re *regexp.Regexp, text string) {
	for _, mm := range re.FindAllStringSubmatch(text, -1) {
		if len(mm) > 1 && mm[1] != "" {
			set[mm[1]] = true
		}
	}
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func addJsExports(set map[string]bool, text string) {
	addAll(set, reExpJs1, text)
	for _, mm := range reExpJs2.FindAllStringSubmatch(text, -1) {
		for _, n := range strings.Split(mm[1], ",") {
			parts := reJsAs.Split(strings.TrimSpace(n), -1)
			if t := strings.TrimSpace(parts[len(parts)-1]); t != "" {
				set[t] = true
			}
		}
	}
	if reExpJsDef.MatchString(text) {
		set["default"] = true
	}
	for _, mm := range reExpCjs.FindAllStringSubmatch(text, -1) {
		for _, n := range strings.Split(mm[1], ",") {
			if t := strings.TrimSpace(strings.SplitN(strings.TrimSpace(n), ":", 2)[0]); t != "" {
				set[t] = true
			}
		}
	}
}

// extractExports lists what a file offers others; rel names the component a .vue or .svelte file is.
func extractExports(text, lang, rel string) []string {
	set := map[string]bool{}
	switch lang {
	case "ts", "js":
		addJsExports(set, text)
	case "vue", "svelte":
		addJsExports(set, text)
		set[strings.TrimSuffix(path.Base(rel), path.Ext(rel))] = true
	case "py":
		for _, mm := range reExpPy.FindAllStringSubmatch(text, -1) {
			if !strings.HasPrefix(mm[1], "_") {
				set[mm[1]] = true
			}
		}
	case "go":
		addAll(set, reExpGo1, text)
		addAll(set, reExpGo2, text)
	case "rs":
		addAll(set, reExpRs, text)
	case "cs":
		addAll(set, reExpCs, text)
	case "java":
		addAll(set, reExpJvm, text)
	case "kotlin":
		addAll(set, reExpJvm, text)
		addAll(set, reExpKtFun, text)
	case "c", "cpp":
		addCExports(set, text)
	}
	return keys(set)
}

func addCExports(set map[string]bool, text string) {
	addAll(set, reExpCType, text)
	for _, mm := range reExpCFunc.FindAllStringSubmatch(text, -1) {
		line := strings.TrimSpace(mm[0])
		if !cKeywords[mm[1]] && !strings.HasPrefix(line, "static ") && !strings.HasPrefix(line, "return ") && !strings.HasPrefix(line, "else ") {
			set[mm[1]] = true
		}
	}
}

func extractImports(text, lang string) []string {
	set := map[string]bool{}
	switch lang {
	case "ts", "js", "vue", "svelte":
		addAll(set, reImpJs1, text)
		addAll(set, reImpJs2, text)
		addAll(set, reImpJs3, text)
	case "py":
		addAll(set, reImpPy1, text)
		addAll(set, reImpPy2, text)
	case "go":
		addAll(set, reImpGo, text)
		addAll(set, reImpGo1, text)
	case "rs":
		addAll(set, reImpRs1, text)
		addAll(set, reImpRs2, text)
	case "cs":
		addAll(set, reImpCs, text)
	case "java", "kotlin":
		addAll(set, reImpJv, text)
	case "c", "cpp":
		addAll(set, reImpC, text)
	}
	return keys(set)
}

// ExportsOf lists what source text exports, for a file named rel.
func ExportsOf(rel, text string) []string {
	if l, ok := langOf(rel); ok {
		return extractExports(text, l, rel)
	}
	return nil
}

// namespaceOf reads the namespace (C#) or package (Java/Kotlin) a file declares.
func namespaceOf(text, lang string) string {
	var re *regexp.Regexp
	switch lang {
	case "cs":
		re = reNamespace
	case "java", "kotlin":
		re = rePackage
	default:
		return ""
	}
	if m := re.FindStringSubmatch(text); len(m) > 1 {
		return m[1]
	}
	return ""
}
