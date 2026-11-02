package api

import (
	"os"
	"sync"
	"testing"
)

// The token is read once and removed from the environment: the agent's shell inherits os.Environ and
// must not be able to authenticate against the local API.
func TestAPITokenDoesNotReachChildProcesses(t *testing.T) {
	t.Setenv("INFUSE_TOKEN", "secret")
	tokenOnce, tokenVal = sync.Once{}, ""
	if got := apiToken(); got != "secret" {
		t.Fatalf("token %q", got)
	}
	if got := apiToken(); got != "secret" {
		t.Fatalf("the token changed between calls: %q", got)
	}
	if v := os.Getenv("INFUSE_TOKEN"); v != "" {
		t.Fatalf("INFUSE_TOKEN stayed in the environment: %q", v)
	}
}

// Routes that read a caller-chosen file, fetch a URL or open a path require the token even on GET.
func TestSensitiveRoutesRequireTheToken(t *testing.T) {
	for _, p := range []string{"/api/file", "/api/frameable", "/api/open"} {
		if !sensitiveRoute(p) {
			t.Errorf("%s is not treated as sensitive", p)
		}
	}
	if sensitiveRoute("/api/map") || sensitiveRoute("/api/journal") {
		t.Error("a plain read route must not require the token")
	}
}

func TestSameOrigin(t *testing.T) {
	const tok = "s3cr3t-token"
	if sameOrigin("127.0.0.1:7470", "", "GET", "", tok) != nil {
		t.Fatal("own GET refused")
	}
	if sameOrigin("127.0.0.1:7470", "http://127.0.0.1:7470", "POST", tok, tok) != nil {
		t.Fatal("own POST refused")
	}
	if sameOrigin("127.0.0.1:7470", "https://evil.example", "POST", tok, tok) == nil {
		t.Fatal("cross-origin accepted")
	}
	if sameOrigin("127.0.0.1:7470", "", "POST", "", tok) == nil {
		t.Fatal("POST without token accepted (text/plain CSRF)")
	}
	if sameOrigin("127.0.0.1:7470", "", "POST", "1", tok) == nil {
		t.Fatal("a local process guessing the marker was accepted")
	}
	if sameOrigin("evil.example:7470", "", "GET", "", tok) == nil {
		t.Fatal("rebinding host accepted")
	}
}
