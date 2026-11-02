package agent

// E1: a changed screen is looked at before the turn ends, at the origin the criteria measure and with
// the HTML the criteria expect. A capture of another page (or of a page that no longer shows what the
// criteria measure) proves nothing about the project's own screen.

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
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
		return M("agent.no_screenshot", "files", strings.Join(firstN(dedupe(l.uiChanged), 6), ", "), "vice", uiChangeVice(l.uiChanged))
	}
	if spec, err := LoadOracleSpec(s.Root); err == nil {
		type requirement struct{ origin, path, html string }
		var reqs []requirement
		for _, c := range spec.Checks {
			if c.Kind == "http" && c.URL != "" {
				reqs = append(reqs, requirement{originOf(c.URL), pathOf(c.URL), c.Expect.Contains})
			}
		}
		if len(reqs) > 0 {
			strong := reqs[:0:0] // a status-only check cannot dilute a check that names the content
			for _, r := range reqs {
				if r.html != "" {
					strong = append(strong, r)
				}
			}
			if len(strong) > 0 {
				reqs = strong
			}
			gotOrigin, gotPath := originOf(l.shotURL), pathOf(l.shotURL)
			st := s.shotState()
			st.mu.Lock()
			html := st.html
			st.mu.Unlock()
			originOK, pathOK := false, false
			for _, r := range reqs {
				if r.origin != gotOrigin {
					continue
				}
				originOK = true
				if r.path != gotPath {
					continue
				}
				pathOK = true
				if r.html == "" || (html != "" && strings.Contains(html, r.html)) {
					return Msg{}
				}
			}
			// The capture must be of the page the criteria measure, and show what they expect from it.
			if !originOK {
				if gotOrigin == "" {
					gotOrigin = "unknown"
				}
				return M("agent.screenshot_origin", "expected", reqs[0].origin, "got", gotOrigin)
			}
			if !pathOK {
				return M("agent.screenshot_path", "expected", reqs[0].path, "got", gotPath)
			}
			return M("agent.screenshot_html", "expected", reqs[0].html)
		}
	}
	return Msg{}
}

// originOf normalizes what browsers treat as the same origin: loopback aliases (localhost, 127.x,
// [::1], 0.0.0.0), default ports and a trailing dot in the host.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	port := u.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	if loopbackHost(host) {
		host = "127.0.0.1"
	}
	if port == "" {
		return scheme + "://" + host
	}
	return scheme + "://" + net.JoinHostPort(host, port)
}

func loopbackHost(host string) bool {
	return host == "localhost" || host == "0.0.0.0" || host == "::1" || host == "[::1]" || strings.HasPrefix(host, "127.")
}

// pathOf is the part of the address a capture must be on: the path, root when empty.
func pathOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := u.EscapedPath()
	if p == "" {
		return "/"
	}
	if len(p) > 1 {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
