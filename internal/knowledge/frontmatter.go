package knowledge

import "strings"

// Frontmatter is the header of a markdown definition file: `key: value` lines between two `---` lines.
// A value may be a comma list, a [bracketed] list, or a YAML list on the lines after its key.
type Frontmatter map[string][]string

// Value is the key's text, its list joined back.
func (f Frontmatter) Value(key string) string { return strings.Join(f[key], ", ") }

// ParseFrontmatter splits a definition file into its header and its body. A file with no header is
// all body.
func ParseFrontmatter(text string) (Frontmatter, string) {
	t := strings.ReplaceAll(text, "\r\n", "\n")
	out := Frontmatter{}
	if !strings.HasPrefix(t, "---\n") {
		return out, text
	}
	end := strings.Index(t[4:], "\n---")
	if end < 0 {
		return out, text
	}
	key := ""
	for _, line := range strings.Split(t[4:4+end], "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && key != "" {
			out[key] = append(out[key], unquote(trimmed[2:]))
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(k)
		out[key] = listValue(key, strings.TrimSpace(v))
	}
	return out, strings.TrimLeft(t[4+end+4:], "\n")
}

// listValue splits the values that are lists; the prose of a description keeps its commas.
func listValue(key, v string) []string {
	if v == "" {
		return nil
	}
	if key != "tools" {
		return []string{unquote(v)}
	}
	var items []string
	for _, it := range strings.Split(strings.Trim(v, "[]"), ",") {
		if s := unquote(it); s != "" {
			items = append(items, s)
		}
	}
	return items
}

func unquote(s string) string { return strings.Trim(strings.TrimSpace(s), `"'`) }
