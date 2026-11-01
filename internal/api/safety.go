package api

// Safety rails around the agent and the local API: path containment by component, capped output,
// secret redaction, private-network fetch refusal, external-content provenance, the daily spend limit
// read from the ledger, the loop breaker, and user approval of launch.json commands.

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"strings"
)

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
// sensitiveRoute names the read-only routes that still require the application token: they read a
// file chosen by the caller, fetch a URL, or hand a path to another application.
func sensitiveRoute(path string) bool {
	switch path {
	case "/api/file", "/api/frameable", "/api/open":
		return true
	}
	return false
}

func sameOrigin(host, origin, method, marker, token string) error {
	h := strings.ToLower(host)
	hn := h
	if i := strings.LastIndex(h, ":"); i > 0 {
		hn = h[:i]
	}
	if hn != "127.0.0.1" && hn != "localhost" && hn != "[::1]" {
		return errors.New("host not allowed")
	}
	if origin != "" && origin != "null" {
		u, err := url.Parse(origin)
		if err != nil || strings.ToLower(u.Host) != h {
			return errors.New("origin not allowed")
		}
	}
	if method != "GET" && method != "HEAD" && method != "OPTIONS" && (token == "" || subtle.ConstantTimeCompare([]byte(marker), []byte(token)) != 1) {
		return errors.New("request without the application token")
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
