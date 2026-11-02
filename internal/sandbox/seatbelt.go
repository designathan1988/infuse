package sandbox

import (
	"fmt"
	"path/filepath"
	"strings"
)

// seatbeltProfile is the macOS write policy: everything readable, writes only under the project and
// the temp folders, and the protected paths denied last, because the last matching rule wins.
func seatbeltProfile(root, tmp string, protected []string) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny file-write*)\n")
	fmt.Fprintf(&b, "(allow file-write* (subpath %s) (subpath %s) (subpath \"/private/tmp\") (subpath \"/private/var/folders\") (literal \"/dev/null\") (literal \"/dev/tty\"))\n", sbQuote(root), sbQuote(tmp))
	deny := []string{filepath.Join(root, ".infuse"), filepath.Join(root, ".git", "hooks"), filepath.Join(root, ".git", "config")}
	for _, rel := range protected {
		deny = append(deny, filepath.Join(root, filepath.FromSlash(rel)))
	}
	b.WriteString("(deny file-write*")
	for _, p := range deny {
		b.WriteString(" (subpath " + sbQuote(p) + ")")
	}
	b.WriteString(")\n")
	return b.String()
}

func sbQuote(p string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(filepath.ToSlash(p)) + `"`
}
