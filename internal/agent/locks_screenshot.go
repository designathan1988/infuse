package agent

// E1: a changed screen is looked at before the turn ends, at the origin the criteria measure and with
// the HTML the criteria expect. A capture of another page (or of a page that no longer shows what the
// criteria measure) proves nothing about the project's own screen.

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"sync"
)

// shotHTMLMax bounds what is kept of a captured page; a DOM larger than this is not a screen.
const shotHTMLMax = 8 << 20

// shotHTMLState keeps the last capture's HTML and its digest, beside the session.
type shotHTMLState struct {
	mu   sync.Mutex
	html string
	hash string
}

var shotHTMLStates sync.Map // *Session → *shotHTMLState

func (s *Session) shotState() *shotHTMLState {
	v, _ := shotHTMLStates.LoadOrStore(s, &shotHTMLState{})
	return v.(*shotHTMLState)
}

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
	s.noteScreenshotAt("", "")
}

func (s *Session) noteScreenshotAt(pageURL, html string) {
	if len(html) > shotHTMLMax {
		html = html[:shotHTMLMax]
	}
	sum := sha256.Sum256([]byte(html))
	st := s.shotState()
	st.mu.Lock()
	st.html, st.hash = html, hex.EncodeToString(sum[:])
	st.mu.Unlock()
	l := s.locks()
	l.mu.Lock()
	l.shotSeq = l.uiChangeSeq
	l.shotURL = pageURL
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
	if spec, err := LoadOracleSpec(s.Root); err == nil {
		wantOrigin, wantHTML := "", ""
		for _, c := range spec.Checks {
			if c.Kind != "http" || c.URL == "" {
				continue
			}
			if wantOrigin == "" {
				wantOrigin = originOf(c.URL)
			}
			if wantHTML == "" && c.Expect.Contains != "" {
				wantHTML = c.Expect.Contains
			}
		}
		// The capture must come from the origin the criteria measure.
		if wantOrigin != "" && originOf(l.shotURL) != wantOrigin {
			got := originOf(l.shotURL)
			if got == "" {
				got = "unknown"
			}
			return M("agent.screenshot_origin", "expected", wantOrigin, "got", got)
		}
		// And it must show the content the criteria measure: the captured HTML is bound to the shot.
		if wantHTML != "" {
			st := s.shotState()
			st.mu.Lock()
			html := st.html
			st.mu.Unlock()
			if html == "" || !strings.Contains(html, wantHTML) {
				return M("agent.screenshot_html", "expected", wantHTML)
			}
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
