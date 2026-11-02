package agent

import (
	"testing"
)

func screenshotSession(t *testing.T, spec string) *Session {
	t.Helper()
	testHome(t)
	root := t.TempDir()
	s := newSession(root)
	writeSpec(t, root, spec)
	s.noteUIChange("src/App.tsx")
	return s
}

// The capture must show the content the criteria measure, not just any page at the right origin.
func TestScreenshotHTMLMustShowWhatTheCriteriaMeasure(t *testing.T) {
	s := screenshotSession(t, `{"checks":[{"id":"page","kind":"http","url":"http://localhost:3000/app","expect":{"contains":"<h1>Dashboard</h1>"}}]}`)
	url := "http://localhost:3000/app"
	s.noteScreenshotAt(url, "<html><body><h1>Dashboard</h1></body></html>")
	if why := s.missingScreenshot(); !why.Empty() {
		t.Fatalf("a matching capture was refused: %q", why.Render("en"))
	}
	s.noteScreenshotAt(url, "<html><body><h1>Other</h1></body></html>")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_html" {
		t.Fatalf("a capture without the measured content passed: %q", why.Render("en"))
	}
	s.noteScreenshotAt(url, "") // a recording keyframe has no HTML
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_html" {
		t.Fatalf("a capture without HTML satisfied a content requirement: %q", why.Render("en"))
	}
}

// Without a content requirement, origin and path bind the capture; loopback aliases are one server.
func TestScreenshotWithoutContentRequirementUsesOriginAndPath(t *testing.T) {
	s := screenshotSession(t, `{"checks":[{"id":"page","kind":"http","url":"http://localhost:4000/app","expect":{"status":200}}]}`)
	for _, url := range []string{"http://localhost:4000/app", "http://127.0.0.1:4000/app", "http://[::1]:4000/app/"} {
		s.noteScreenshotAt(url, "")
		if why := s.missingScreenshot(); !why.Empty() {
			t.Fatalf("a capture at the measured page (%s) was refused: %q", url, why.Render("en"))
		}
	}
	s.noteScreenshotAt("http://localhost:4000/", "")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_path" {
		t.Fatalf("a capture of another page at the same origin passed: %q", why.Render("en"))
	}
	s.noteScreenshotAt("http://localhost:9999/app", "")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_origin" {
		t.Fatalf("a capture at another origin passed: %q", why.Render("en"))
	}
}

// A check that names the content governs the capture even when another check only measures a status.
func TestScreenshotPrefersTheContentCheck(t *testing.T) {
	s := screenshotSession(t, `{"checks":[
		{"id":"root","kind":"http","url":"http://localhost:3000/","expect":{"status":200}},
		{"id":"page","kind":"http","url":"http://localhost:3000/checkout","expect":{"contains":"<h1>Checkout</h1>"}}]}`)
	s.noteScreenshotAt("http://localhost:3000/", "<html><body><h1>Checkout</h1></body></html>")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_path" {
		t.Fatalf("a status-only check excused another page: %q", why.Render("en"))
	}
	s.noteScreenshotAt("http://localhost:3000/checkout", "<html><body><h1>Other</h1></body></html>")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_html" {
		t.Fatalf("a capture without the measured content passed: %q", why.Render("en"))
	}
	s.noteScreenshotAt("http://127.0.0.1:3000/checkout", "<html><body><h1>Checkout</h1></body></html>")
	if why := s.missingScreenshot(); !why.Empty() {
		t.Fatalf("a matching capture through a loopback alias was refused: %q", why.Render("en"))
	}
}
