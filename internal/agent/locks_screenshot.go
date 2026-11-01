package agent

// E1: a changed screen is looked at before the turn ends, at the origin the criteria measure.

import (
	"net/url"
	"strings"
)

// noteUIChange and noteScreenshot feed the obligation below.
func (s *Session) noteUIChange(rel string) {
	if !isUIFile(rel) || isTestPath(rel) {
		return
	}
	l := s.locks()
	l.mu.Lock()
	l.uiChangeSeq++
	l.uiChanged = append(l.uiChanged, rel)
	l.mu.Unlock()
}

func (s *Session) noteScreenshot() {
	s.noteScreenshotAt("")
}

func (s *Session) noteScreenshotAt(url string) {
	l := s.locks()
	l.mu.Lock()
	l.shotSeq = l.uiChangeSeq
	l.shotURL = url
	l.mu.Unlock()
}

// missingScreenshot: the turn changed what a screen shows and nobody looked at it afterwards.
func (s *Session) missingScreenshot() Msg {
	l := s.locks()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.uiChangeSeq == 0 {
		return Msg{}
	}
	if l.shotSeq < l.uiChangeSeq {
		return M("agent.no_screenshot", "files", strings.Join(firstN(dedupe(l.uiChanged), 6), ", "))
	}
	// A capture only counts for the origin the criteria measure: a screenshot of another page proves
	// nothing about the project's own screen.
	if spec, err := LoadOracleSpec(s.Root); err == nil {
		want := ""
		for _, c := range spec.Checks {
			if c.Kind == "http" && c.URL != "" {
				want = originOf(c.URL)
				break
			}
		}
		if want != "" && originOf(l.shotURL) != want {
			got := originOf(l.shotURL)
			if got == "" {
				got = "unknown"
			}
			return M("agent.screenshot_origin", "expected", want, "got", got)
		}
	}
	return Msg{}
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}
