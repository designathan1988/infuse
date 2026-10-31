package security

import (
	"strings"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	in := "key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123 and AKIAABCDEFGHIJKLMNOP and api_key=supersecretvalue123 ok"
	out := RedactSecrets(in)
	for _, leak := range []string{"abcdefghijklmnop", "AKIAABCDEFGHIJKLMNOP", "supersecretvalue123"} {
		if strings.Contains(out, leak) {
			t.Fatalf("leak %q in %q", leak, out)
		}
	}
	if !strings.Contains(out, "api_key=") || !strings.Contains(out, "ok") {
		t.Fatalf("over-redacted: %q", out)
	}
}

func TestPrivateTargets(t *testing.T) {
	for _, u := range []string{"http://127.0.0.1:7470/api/settings", "http://localhost/x", "http://169.254.169.254/latest", "file:///etc/passwd", "http://10.0.0.5/"} {
		if bad, _ := PrivateTarget(u); !bad {
			t.Fatalf("%s allowed", u)
		}
	}
}
