package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithinByComponent(t *testing.T) {
	root := filepath.Join("C:", "proj")
	if within(root, filepath.Join("C:", "proj-2", "x")) {
		t.Fatal("sibling with the same prefix accepted")
	}
	if !within(root, filepath.Join(root, "a", "b")) || !within(root, root) {
		t.Fatal("inside rejected")
	}
	if within(root, filepath.Join(root, "..", "other")) {
		t.Fatal("escape accepted")
	}
}

func TestCapWriterKeepsHeadAndTail(t *testing.T) {
	w := newCapWriter(10)
	for i := 0; i < 1000; i++ {
		_, _ = w.Write([]byte("0123456789"))
	}
	s := w.String()
	if len(s) > 200 || !strings.HasPrefix(s, "0123456789") || !strings.HasSuffix(s, "0123456789") || !strings.Contains(s, "omitted") {
		t.Fatalf("%q", s)
	}
}

func TestRedactSecrets(t *testing.T) {
	in := "key sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123 and AKIAABCDEFGHIJKLMNOP and api_key=supersecretvalue123 ok"
	out := redactSecrets(in)
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
		if bad, _ := privateTarget(u); !bad {
			t.Fatalf("%s allowed", u)
		}
	}
}

func TestLoopBreaker(t *testing.T) {
	var b loopBreaker
	in := json.RawMessage(`{"command":"make"}`)
	if b.record("Bash", in, true) != nil || b.record("Bash", in, true) != nil {
		t.Fatal("tripped too early")
	}
	if b.record("Bash", in, true) == nil {
		t.Fatal("same failure x3 must trip")
	}
	var c loopBreaker
	for i := 0; i < 7; i++ {
		if c.record("Bash", json.RawMessage(`{"i":`+string(rune('0'+i))+`}`), true) != nil {
			t.Fatal("tripped before 8")
		}
	}
	if c.record("Read", json.RawMessage(`{}`), true) == nil {
		t.Fatal("8 consecutive failures must trip")
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

func TestLaunchNeedsApproval(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	c := launchConfig{Name: "web", RuntimeExecutable: "npm", RuntimeArgs: []string{"run", "dev"}}
	if launchApproved(root, c) {
		t.Fatal("approved by default")
	}
	_ = approveLaunch(root, c)
	if !launchApproved(root, c) {
		t.Fatal("approval not kept")
	}
	c.RuntimeArgs = []string{"run", "evil"}
	if launchApproved(root, c) {
		t.Fatal("a changed command kept the approval")
	}
}
