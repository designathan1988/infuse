// Package lang holds language- and filesystem-shape knowledge shared by the filter, the checks and the
// agent: glob compilation, extension tables, code metrics and test-path rules.
package lang

import (
	"regexp"
	"strings"
)

// GlobToRegex compiles a glob (with **, ?, {a,b}) into a regular expression over posix paths.
func GlobToRegex(pat string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pat); i++ {
		c := pat[i]
		switch {
		case c == '*' && i+1 < len(pat) && pat[i+1] == '*':
			b.WriteString(".*")
			i++
			if i+1 < len(pat) && pat[i+1] == '/' {
				i++
				b.WriteString("(?:/|)")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			j := strings.IndexByte(pat[i:], '}')
			if j > 0 {
				alts := strings.Split(pat[i+1:i+j], ",")
				for k := range alts {
					alts[k] = regexp.QuoteMeta(alts[k])
				}
				b.WriteString("(?:" + strings.Join(alts, "|") + ")")
				i += j
				continue
			}
			b.WriteString(`\{`)
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return regexp.MustCompile("^$")
	}
	return re
}
