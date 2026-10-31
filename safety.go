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
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

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
