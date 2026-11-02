package api

import "testing"

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
