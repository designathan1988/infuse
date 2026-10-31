package main

// Safety rails around the agent and the local API: path containment by component, capped output,
// secret redaction, private-network fetch refusal, external-content provenance, the daily spend limit
// read from the ledger, the loop breaker, and user approval of launch.json commands.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// within reports whether p is root or inside it, comparing whole path components (C:\proj-2 is not in C:\proj).
func within(root, p string) bool {
	if root == "" || p == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// capWriter keeps the first and last `limit` bytes of a stream, so a command that prints gigabytes
// cannot exhaust memory; what was dropped is reported in the middle.
type capWriter struct {
	limit   int
	head    []byte
	tail    []byte
	dropped int64
}

func newCapWriter(limit int) *capWriter { return &capWriter{limit: limit} }

func (w *capWriter) Write(p []byte) (int, error) {
	n := len(p)
	if room := w.limit - len(w.head); room > 0 {
		k := min(room, len(p))
		w.head = append(w.head, p[:k]...)
		p = p[k:]
	}
	if len(p) > 0 {
		w.tail = append(w.tail, p...)
		if over := len(w.tail) - w.limit; over > 0 {
			w.dropped += int64(over)
			w.tail = append(w.tail[:0], w.tail[over:]...)
		}
	}
	return n, nil
}

func (w *capWriter) String() string {
	if w.dropped == 0 {
		return string(w.head) + string(w.tail)
	}
	return string(w.head) + "\n\n" + M("safety.omitted", "bytes", w.dropped).String() + "\n\n" + string(w.tail)
}

// secret patterns: provider keys, cloud credentials, tokens, private keys, key=value assignments.
var reSecrets = []*regexp.Regexp{
	regexp.MustCompile(`sk-ant-[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`sk-(?:proj-|live-)?[A-Za-z0-9_\-]{20,}`),
	regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
	regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{30,}`),
	regexp.MustCompile(`github_pat_[A-Za-z0-9_]{30,}`),
	regexp.MustCompile(`xox[abprs]-[A-Za-z0-9\-]{10,}`),
	regexp.MustCompile(`AIza[0-9A-Za-z_\-]{30,}`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b((?:api[_-]?key|secret|token|password|passwd|pwd|access[_-]?key)["']?\s*[:=]\s*["']?)([^\s"',;]{8,})`),
}

// redactSecrets replaces credentials in text that leaves the tool (to the model, the journal, the UI).
func redactSecrets(s string) string {
	if s == "" {
		return s
	}
	for i, re := range reSecrets {
		if i == len(reSecrets)-1 {
			s = re.ReplaceAllString(s, "${1}[REDACTED]")
		} else {
			s = re.ReplaceAllString(s, "[REDACTED]")
		}
	}
	return s
}

// privateTarget reports whether a URL points at this machine or the private network (SSRF guard), with
// the reason in English (privateTargetMsg gives the message itself).
func privateTarget(raw string) (bool, string) {
	bad, why := privateTargetMsg(raw)
	return bad, why.String()
}

// privateTargetMsg reports whether a URL points at this machine or the private network, and why.
func privateTargetMsg(raw string) (bool, Msg) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return true, M("safety.bad_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return true, M("safety.scheme", "scheme", u.Scheme)
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return true, M("safety.local", "host", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false, Msg{}
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.Equal(net.ParseIP("169.254.169.254")) {
			return true, M("safety.internal_ip", "ip", ip.String())
		}
	}
	return false, Msg{}
}

// externalContent labels fetched text as data from a source, never instructions (prompt-injection hygiene).
func externalContent(source, text string) string {
	return fmt.Sprintf("<external-content source=%q fetched=%q>\n%s\n</external-content>\n", source, time.Now().Format(time.RFC3339), text) + M("safety.external").String()
}

// todaySpend sums today's cost in the usage ledger (all sessions, all projects).
func todaySpend() float64 {
	today := time.Now().Format("2006-01-02")
	total := 0.0
	for _, e := range readJSONL(usageFile()) {
		ts, _ := e["ts"].(float64)
		if time.UnixMilli(int64(ts)).Format("2006-01-02") == today {
			c, _ := e["cost"].(float64)
			total += c
		}
	}
	return total
}

// loopBreaker stops a turn that keeps failing: the same failing call three times, or eight failures in a row.
type loopBreaker struct {
	same        map[string]int
	consecutive int
}

// record counts a call's outcome; the error it returns when the breaker trips carries the message
// (msgOf), so a caller with a session shows it to the user in their language.
func (b *loopBreaker) record(name string, input json.RawMessage, isErr bool) error {
	if !isErr {
		b.consecutive = 0
		return nil
	}
	if b.same == nil {
		b.same = map[string]int{}
	}
	key := name + "|" + string(input)
	b.same[key]++
	b.consecutive++
	if b.same[key] >= 3 {
		return errMsg(M("breaker.same", "tool", name))
	}
	if b.consecutive >= 8 {
		return errMsg(M("breaker.many"))
	}
	return nil
}

// ---------------------------------------------------------------- launch.json approval

// A dev-server command lives in the project, where the agent can write. It runs only after the user
// approves that exact command (executable, arguments, folder, environment) — approvals live in
// ~/.infuse, out of the agent's reach.
var launchMu sync.Mutex

func launchFingerprint(root string, c launchConfig) string {
	b, _ := json.Marshal([]any{filepath.Clean(root), c.Name, c.RuntimeExecutable, c.RuntimeArgs, c.Cwd, c.Env})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func approvalsFile() string { return filepath.Join(InfuseHome(), "approved-launch.json") }

func launchApproved(root string, c launchConfig) bool {
	launchMu.Lock()
	defer launchMu.Unlock()
	var m map[string]bool
	_ = readJSON(approvalsFile(), &m)
	return m[launchFingerprint(root, c)]
}

func approveLaunch(root string, c launchConfig) error {
	launchMu.Lock()
	defer launchMu.Unlock()
	m := map[string]bool{}
	_ = readJSON(approvalsFile(), &m)
	m[launchFingerprint(root, c)] = true
	return writeJSON(approvalsFile(), m)
}

func launchCommandLine(c launchConfig) string {
	parts := append([]string{c.RuntimeExecutable}, c.RuntimeArgs...)
	s := strings.Join(parts, " ")
	if c.Cwd != "" {
		s += "   (folder: " + c.Cwd + ")"
	}
	for k, v := range c.Env {
		s += "   " + k + "=" + v
	}
	return s
}

// ---------------------------------------------------------------- local API: same-origin only

// apiToken is this run's key to the API: random, or INFUSE_TOKEN when set (development with `infuse serve`).
func apiToken() string {
	if t := os.Getenv("INFUSE_TOKEN"); t != "" {
		return t
	}
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sameOrigin accepts a request only from the app itself: the Host must be this server's loopback
// address (defeats DNS rebinding), a browser Origin must be that same origin, and state-changing
// requests must carry X-Infuse with this run's token — which a cross-site form cannot set and a local
// process (the agent's Bash) does not know.
func sameOrigin(host, origin, method, marker, token string) error {
	h := strings.ToLower(host)
	hn := h
	if i := strings.LastIndex(h, ":"); i > 0 {
		hn = h[:i]
	}
	if hn != "127.0.0.1" && hn != "localhost" && hn != "[::1]" {
		return errors.New("host não permitido")
	}
	if origin != "" && origin != "null" {
		u, err := url.Parse(origin)
		if err != nil || strings.ToLower(u.Host) != h {
			return errors.New("origem não permitida")
		}
	}
	if method != "GET" && method != "HEAD" && method != "OPTIONS" && (token == "" || subtle.ConstantTimeCompare([]byte(marker), []byte(token)) != 1) {
		return errors.New("requisição sem o token do aplicativo")
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// randomHex returns n random bytes as hex (identifiers that must not collide).
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
