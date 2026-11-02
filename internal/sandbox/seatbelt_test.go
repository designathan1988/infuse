package sandbox

import (
	"strings"
	"testing"
)

// The last matching rule wins in Seatbelt: the protected paths must come after the project's grant.
func TestSeatbeltDeniesProtectedPathsAfterTheGrant(t *testing.T) {
	p := seatbeltProfile("/w/proj", "/private/tmp/x", []string{".mcp.json"})
	allow, deny := strings.Index(p, "(allow file-write*"), strings.LastIndex(p, "(deny file-write*")
	if allow < 0 || deny < allow {
		t.Fatalf("order:\n%s", p)
	}
	for _, want := range []string{`"/w/proj/.infuse"`, `"/w/proj/.git/hooks"`, `"/w/proj/.git/config"`, `"/w/proj/.mcp.json"`} {
		if !strings.Contains(p[deny:], want) {
			t.Fatalf("%s is not denied:\n%s", want, p)
		}
	}
}

func TestSeatbeltQuotesPaths(t *testing.T) {
	if got := sbQuote(`/a "b"`); got != `"/a \"b\""` {
		t.Fatal(got)
	}
}
