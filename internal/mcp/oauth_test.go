package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// authServer is an authorization server and an MCP endpoint in one: /mcp answers only to a valid bearer.
type authServer struct {
	*httptest.Server
	mu         sync.Mutex
	challenge  string
	valid      map[string]bool
	registered atomic.Int32
	refreshed  atomic.Int32
	expires    int
}

func newAuthServer(t *testing.T) *authServer {
	t.Helper()
	a := &authServer{valid: map[string]bool{}, expires: 3600}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"authorization_endpoint":"%[1]s/authorize","token_endpoint":"%[1]s/token","registration_endpoint":"%[1]s/register"}`, a.URL)
	})
	mux.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		a.registered.Add(1)
		var in struct {
			Method string `json:"token_endpoint_auth_method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Method != "none" {
			http.Error(w, "public clients only", 400)
			return
		}
		fmt.Fprint(w, `{"client_id":"client-1"}`)
	})
	mux.HandleFunc("/token", a.token)
	mux.HandleFunc("/mcp", a.mcp)
	a.Server = httptest.NewServer(mux)
	t.Cleanup(a.Close)
	return a
}

func (a *authServer) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	a.mu.Lock()
	defer a.mu.Unlock()
	access := ""
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		if pkceChallenge(r.Form.Get("code_verifier")) != a.challenge || r.Form.Get("code") != "the-code" || r.Form.Get("client_id") != "client-1" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		access = "access-1"
	case "refresh_token":
		a.refreshed.Add(1)
		if r.Form.Get("refresh_token") != "refresh-1" {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		access = "access-2"
	}
	a.valid[access] = true
	fmt.Fprintf(w, `{"access_token":%q,"refresh_token":"refresh-1","expires_in":%d}`, access, a.expires)
}

func (a *authServer) mcp(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	ok := a.valid[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
	a.mu.Unlock()
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var m RPCMessage
	_ = json.NewDecoder(r.Body).Decode(&m)
	if m.ID != nil {
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%d,"result":{"tools":[]}}`, *m.ID)
	}
}

func (a *authServer) revoke(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.valid, token)
}

func withMemorySecrets(t *testing.T) {
	t.Helper()
	old := config.SecretBackend
	config.SecretBackend = config.MemSecrets{}
	t.Cleanup(func() { config.SecretBackend = old })
}

// login walks the flow the way a browser would.
func login(t *testing.T, a *authServer, cfg ServerConfig) {
	t.Helper()
	done := make(chan error, 1)
	u, err := BeginOAuth(context.Background(), "root", "project", "srv", cfg, func(e error) { done <- e })
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("client_id") != "client-1" || q.Get("resource") != cfg.URL || parsed.Path != "/authorize" {
		t.Fatal(u)
	}
	a.mu.Lock()
	a.challenge = q.Get("code_challenge")
	a.mu.Unlock()
	redirect := q.Get("redirect_uri")
	if !strings.HasPrefix(redirect, "http://127.0.0.1:") {
		t.Fatal("the redirect is a loopback address:", redirect)
	}
	resp, err := http.Get(redirect + "?code=the-code&state=forged")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatal("a forged state was served:", resp.Status)
	}
	select {
	case e := <-done:
		t.Fatal("a forged state ended the wait:", e)
	case <-time.After(30 * time.Millisecond):
	}
	resp, err = http.Get(redirect + "?code=the-code&state=" + url.QueryEscape(q.Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestOAuthLoginStoresTokensInTheVaultOnly(t *testing.T) {
	withMemorySecrets(t)
	a := newAuthServer(t)
	cfg := ServerConfig{Type: "http", URL: a.URL + "/mcp"}
	c := NewClient("srv", "project", cfg)
	if err := c.Start(context.Background(), "root"); err == nil {
		t.Fatal("a server that answers 401 started")
	}
	if !strings.Contains(c.err.String(), "sign in") {
		t.Fatal("the failure must say what to do:", c.err.String())
	}
	login(t, a, cfg)
	if a.registered.Load() != 1 {
		t.Fatal("registration:", a.registered.Load())
	}
	if !HasOAuthLogin("root", "project", "srv", cfg) {
		t.Fatal("no login in the vault")
	}
	c = NewClient("srv", "project", cfg)
	if err := c.Start(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	// a token revoked on the server: a 401 trades the refresh token, once, and the call goes through
	a.revoke("access-1")
	if _, err := c.Call(context.Background(), "tools/list", nil); err != nil {
		t.Fatal(err)
	}
	if a.refreshed.Load() != 1 {
		t.Fatal("refreshes:", a.refreshed.Load())
	}
}

func TestExpiredTokenRefreshesBeforeTheCall(t *testing.T) {
	withMemorySecrets(t)
	a := newAuthServer(t)
	a.expires = 1 // inside the skew: expired as it arrives
	cfg := ServerConfig{Type: "http", URL: a.URL + "/mcp"}
	login(t, a, cfg)
	a.expires = 3600
	c := NewClient("srv", "project", cfg)
	if err := c.Start(context.Background(), "root"); err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if a.refreshed.Load() == 0 {
		t.Fatal("an expired token was sent")
	}
}

func TestOAuthNeedsAnHTTPServerAndAClient(t *testing.T) {
	withMemorySecrets(t)
	if _, err := BeginOAuth(context.Background(), "root", "project", "srv", ServerConfig{Command: "x"}, nil); err == nil {
		t.Fatal("a stdio server signed in")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "oauth-authorization-server") {
			fmt.Fprint(w, `{"authorization_endpoint":"http://x/a","token_endpoint":"http://x/t"}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if _, err := BeginOAuth(context.Background(), "root", "project", "srv", ServerConfig{Type: "http", URL: srv.URL}, nil); err == nil {
		t.Fatal("a server with no registration and no client id")
	}
	u, err := BeginOAuth(context.Background(), "root", "project", "srv", ServerConfig{Type: "http", URL: srv.URL, ClientID: "mine"}, nil)
	if err != nil || !strings.Contains(u, "client_id=mine") {
		t.Fatal(u, err)
	}
}
