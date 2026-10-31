package agent

// Locks on how work is shown to be done (docs/FALHAS-DE-AGENTES.md E1, C3, B11, D6): a changed screen is
// looked at before the turn ends; a large removal is grounded in the diagnosis or the plan; every web
// address the agent cites was fetched; a package exists in its registry before it is installed. All of it
// is decided on paths, file kinds, addresses and registry answers — no words.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// lockState is what these locks remember about a session's turn (kept beside the session).
type lockState struct {
	mu          sync.Mutex
	uiChangeSeq int
	shotSeq     int
	uiChanged   []string
	fetched     map[string]bool
}

var lockStates sync.Map // *Session → *lockState

func (s *Session) locks() *lockState {
	v, _ := lockStates.LoadOrStore(s, &lockState{fetched: map[string]bool{}})
	return v.(*lockState)
}

// resetTurnLocks starts a new turn: what a screen showed must be looked at again; fetched addresses stay.
func (s *Session) resetTurnLocks() {
	l := s.locks()
	l.mu.Lock()
	l.uiChangeSeq, l.shotSeq, l.uiChanged = 0, 0, nil
	l.mu.Unlock()
}

// ---------------------------------------------------------------- E1: a changed screen is looked at

var uiExts = map[string]bool{".css": true, ".scss": true, ".less": true, ".html": true, ".htm": true, ".jsx": true, ".tsx": true, ".vue": true, ".svelte": true, ".astro": true}

// isUIFile reports whether a change to rel changes what a screen shows.
func isUIFile(rel string) bool {
	rel = filepath.ToSlash(rel)
	if uiExts[strings.ToLower(filepath.Ext(rel))] {
		return true
	}
	for _, d := range []string{"/ui/", "/components/", "/pages/", "/views/", "/screens/", "/styles/"} {
		if strings.Contains("/"+rel, d) && jsLikeExts[strings.ToLower(filepath.Ext(rel))] {
			return true
		}
	}
	return false
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
	l := s.locks()
	l.mu.Lock()
	l.shotSeq = l.uiChangeSeq
	l.mu.Unlock()
}

// missingScreenshot: the turn changed what a screen shows and nobody looked at it afterwards.
func (s *Session) missingScreenshot() Msg {
	l := s.locks()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.uiChangeSeq == 0 || l.shotSeq >= l.uiChangeSeq {
		return Msg{}
	}
	return M("ui.no_screenshot", "files", strings.Join(firstN(dedupe(l.uiChanged), 6), ", "))
}

// ---------------------------------------------------------------- C3: large removals are grounded

// largeRemoval: a write that removes most of an existing file must be named by the plan or diagnosis.
func (s *Session) largeRemoval(rel, before, after string) Msg {
	nb := strings.Count(before, "\n") + 1
	if before == "" || nb < 30 {
		return Msg{}
	}
	removed := len(addedLines(after, before)) // lines of before that after no longer has
	if float64(removed)/float64(nb) < 0.6 {
		return Msg{}
	}
	name := filepath.ToSlash(rel)
	base := filepath.Base(rel)
	s.Mu.Lock()
	grounded := false
	for _, t := range s.Tasks {
		if t.Status != "done" && (strings.Contains(t.Text, base) || strings.Contains(t.Proof, base)) {
			grounded = true
		}
	}
	if n := len(s.Diagnoses); n > 0 {
		d := s.Diagnoses[n-1]
		if strings.Contains(d.Approach, base) || strings.Contains(d.RootCause, base) {
			grounded = true
		}
	}
	s.Mu.Unlock()
	if grounded {
		return Msg{}
	}
	return M("work.large_removal", "file", name, "removed", removed, "total", nb)
}

// ---------------------------------------------------------------- B11: cited addresses were fetched

var reURLCited = regexp.MustCompile(`https?://[^\s<>"'\x60)\]]+`)

// noteFetched records an address the agent actually read (WebFetch, Browser, search results).
func (s *Session) noteFetched(text string) {
	l := s.locks()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, u := range reURLCited.FindAllString(text, -1) {
		l.fetched[normURL(u)] = true
	}
}

func normURL(u string) string {
	u = strings.TrimRight(u, ".,;:!?")
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	p.Fragment = ""
	return strings.TrimSuffix(strings.ToLower(p.Host)+p.EscapedPath(), "/")
}

// unfetchedCitations lists the addresses in text that the agent never fetched nor saw in a result.
func (s *Session) unfetchedCitations(text string) []string {
	l := s.locks()
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, u := range reURLCited.FindAllString(text, -1) {
		n := normURL(u)
		host := strings.SplitN(n, "/", 2)[0]
		if h, _, ok := strings.Cut(host, ":"); ok {
			host = h
		}
		if !l.fetched[n] && !localHosts[host] && !contains(out, u) {
			out = append(out, u)
		}
	}
	return out
}

// ---------------------------------------------------------------- D6: packages exist before install

var (
	reNpmInstall = regexp.MustCompile(`\b(?:npm\s+(?:i|install|add)|pnpm\s+(?:add|i|install)|yarn\s+add|bun\s+add)\s+([^\n;&|]+)`)
	rePipInstall = regexp.MustCompile(`\b(?:pip3?|uv\s+pip|python3?\s+-m\s+pip)\s+install\s+([^\n;&|]+)`)
	reCargoAdd   = regexp.MustCompile(`\bcargo\s+add\s+([^\n;&|]+)`)
)

type pkgRef struct{ registry, name string }

// packagesInCommand lists the packages a command installs.
func packagesInCommand(cmd string) []pkgRef {
	var out []pkgRef
	add := func(reg, list string) {
		for _, w := range strings.Fields(list) {
			if isLocalPackageArg(w) {
				continue
			}
			name := w
			if reg == "npm" {
				if i := strings.LastIndex(w, "@"); i > 0 {
					name = w[:i]
				}
			} else if i := strings.IndexAny(w, "=<>!~["); i > 0 {
				name = w[:i]
			}
			if name != "" {
				out = append(out, pkgRef{reg, name})
			}
		}
	}
	for _, m := range reNpmInstall.FindAllStringSubmatch(cmd, -1) {
		add("npm", m[1])
	}
	for _, m := range rePipInstall.FindAllStringSubmatch(cmd, -1) {
		if !strings.Contains(m[1], "-r ") {
			add("pypi", m[1])
		}
	}
	for _, m := range reCargoAdd.FindAllStringSubmatch(cmd, -1) {
		add("crates", m[1])
	}
	return out
}

// isLocalPackageArg: an option, a path, an archive or a repository, not a registry name.
func isLocalPackageArg(w string) bool {
	l := strings.ToLower(w)
	if strings.HasPrefix(l, "-") || strings.HasPrefix(l, ".") || strings.HasPrefix(l, "/") || strings.Contains(l, `\`) || strings.Contains(l, "://") {
		return true
	}
	for _, p := range []string{"git+", "github:", "gitlab:", "file:", "link:", "workspace:"} {
		if strings.HasPrefix(l, p) {
			return true
		}
	}
	for _, e := range []string{".tgz", ".tar.gz", ".whl", ".zip"} {
		if strings.HasSuffix(l, e) {
			return true
		}
	}
	return strings.Contains(l, "/") && !strings.HasPrefix(l, "@") // owner/repo shorthand
}

// registryLookup is replaced in tests.
var registryLookup = func(ctx context.Context, p pkgRef) (exists bool, created time.Time, err error) {
	var u string
	switch p.registry {
	case "npm":
		u = "https://registry.npmjs.org/" + url.PathEscape(p.name)
	case "pypi":
		u = "https://pypi.org/pypi/" + url.PathEscape(p.name) + "/json"
	case "crates":
		u = "https://crates.io/api/v1/crates/" + url.PathEscape(p.name)
	default:
		return true, time.Time{}, nil
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("User-Agent", "infuse/"+version)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return true, time.Time{}, err // the registry could not be asked: do not block on the network
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return false, time.Time{}, nil
	}
	var body struct {
		Time struct {
			Created time.Time `json:"created"`
		} `json:"time"` // npm
		Crate struct {
			Created time.Time `json:"created_at"`
		} `json:"crate"` // crates.io
		Releases map[string][]struct {
			Upload time.Time `json:"upload_time_iso_8601"`
		} `json:"releases"` // pypi
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	created = body.Time.Created
	if created.IsZero() {
		created = body.Crate.Created
	}
	for _, rel := range body.Releases {
		for _, f := range rel {
			if created.IsZero() || f.Upload.Before(created) {
				created = f.Upload
			}
		}
	}
	return true, created, nil
}

// unknownPackages refuses packages that do not exist and asks about very new ones (a name an LLM
// invented may have been registered by an attacker).
func unknownPackages(ctx context.Context, cmd string) Msg {
	var missing, fresh []string
	for _, p := range packagesInCommand(cmd) {
		ok, created, err := registryLookup(ctx, p)
		if err != nil {
			continue
		}
		if !ok {
			missing = append(missing, fmt.Sprintf("%s (%s)", p.name, p.registry))
		} else if !created.IsZero() && time.Since(created) < 30*24*time.Hour {
			fresh = append(fresh, fmt.Sprintf("%s (%s, %s)", p.name, p.registry, created.Format("2006-01-02")))
		}
	}
	if len(missing) > 0 {
		return M("pkg.missing", "packages", strings.Join(missing, ", "))
	}
	if len(fresh) > 0 {
		return M("pkg.fresh", "packages", strings.Join(fresh, ", "))
	}
	return Msg{}
}

// ---------------------------------------------------------------- the turn's end and the work's state

// workObligation: what the work itself still owes before the turn may end (a changed screen not looked
// at; web addresses cited that were never fetched).
func (s *Session) workObligation() Msg {
	if m := s.missingScreenshot(); !m.Empty() {
		return m
	}
	var text strings.Builder
	s.Mu.Lock()
	start := 0
	if s.Turn >= 1 && s.Turn-1 < len(s.TurnStarts) {
		start = s.TurnStarts[s.Turn-1]
	}
	for _, m := range s.Messages[min(start, len(s.Messages)):] {
		if m.Role != "assistant" {
			continue
		}
		for _, b := range m.Content {
			if b.Type == "text" {
				text.WriteString(b.Text + "\n")
			}
		}
	}
	s.Mu.Unlock()
	if urls := s.unfetchedCitations(text.String()); len(urls) > 0 {
		return M("claims.unfetched", "urls", strings.Join(firstN(urls, 6), ", "))
	}
	return Msg{}
}

// workStatePrompt keeps the state of the work in every request — the diagnosis, the plan, the user's
// prohibitions — so that a compaction or a long turn never loses what was decided.
func (s *Session) workStatePrompt() string {
	var b strings.Builder
	b.WriteString("## State of the work (kept by infuse; it survives compaction)\n")
	s.Mu.Lock()
	if n := len(s.Diagnoses); n > 0 {
		d := s.Diagnoses[n-1]
		fmt.Fprintf(&b, "Current diagnosis (turn %d):\n- problem: %s\n- root cause: %s\n- approach: %s\n- why it treats the cause: %s\n", d.Turn, d.Problem, d.RootCause, d.Approach, d.WhyCause)
		for _, e := range d.Evidence {
			fmt.Fprintf(&b, "  - evidence %s: “%s”\n", e.File, truncate(e.Quote, 200))
		}
	} else {
		b.WriteString("No diagnosis registered yet in this session.\n")
	}
	if len(s.Tasks) > 0 {
		b.WriteString("Plan:\n")
		for _, t := range s.Tasks {
			fmt.Fprintf(&b, "- [%s] %s\n", t.Status, t.Text)
		}
	}
	s.Mu.Unlock()
	var bans []string
	for _, x := range s.Bans() {
		if !x.Off {
			bans = append(bans, fmt.Sprintf("- %s %q — the user's words: “%s”", x.Kind, x.Value, x.Quote))
		}
	}
	if len(bans) > 0 {
		b.WriteString("The user's prohibitions in force (infuse refuses calls that break them):\n" + strings.Join(bans, "\n") + "\n")
	}
	return b.String()
}
