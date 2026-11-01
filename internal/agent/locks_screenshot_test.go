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

// Without a content requirement, the origin remains the only binding.
func TestScreenshotWithoutContentRequirementUsesOriginOnly(t *testing.T) {
	s := screenshotSession(t, `{"checks":[{"id":"page","kind":"http","url":"http://localhost:4000/","expect":{"status":200}}]}`)
	s.noteScreenshotAt("http://localhost:4000/", "")
	if why := s.missingScreenshot(); !why.Empty() {
		t.Fatalf("a capture at the measured origin was refused: %q", why.Render("en"))
	}
	s.noteScreenshotAt("http://localhost:9999/", "")
	if why := s.missingScreenshot(); why.ID != "agent.screenshot_origin" {
		t.Fatalf("a capture at another origin passed: %q", why.Render("en"))
	}
}
