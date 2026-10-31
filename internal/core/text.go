package core

import (
	"sort"
	"strings"
	"unicode/utf8"
)

func Truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func FirstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

func FirstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func NormSpace(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func SplitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

func SnippetAround(text string, k, n int) string {
	from, to := max(0, k-40), min(len(text), k+n+40)
	for from > 0 && !utf8.RuneStart(text[from]) {
		from--
	}
	for to < len(text) && !utf8.RuneStart(text[to]) {
		to++
	}
	s := strings.Join(strings.Fields(text[from:to]), " ")
	if from > 0 {
		s = "…" + s
	}
	if to < len(text) {
		s += "…"
	}
	return s
}

func Dedupe(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func Contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func SortedKeys[M ~map[string]V, V any](m M) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func FirstNonZero(a, b float64) float64 {
	if a != 0 {
		return a
	}
	return b
}

// TextOf joins the text blocks of a message.
func TextOf(m Message) string {
	var b strings.Builder
	for _, c := range m.Content {
		if c.Type == "text" {
			if b.Len() > 0 {
				b.WriteString("\n")
			}
			b.WriteString(c.Text)
		}
	}
	return b.String()
}
