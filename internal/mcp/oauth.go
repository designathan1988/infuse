package mcp

// OAuth 2.1 with PKCE for HTTP servers. The user starts the flow from the screen: infuse discovers the
// authorization server (RFC 9728, RFC 8414), registers itself when the server allows it (RFC 7591), and
// waits on a one-shot loopback listener for the redirect (RFC 8252). Tokens live in the OS vault, never
// in a file of the project or of infuse.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	oauthLoginWait = 5 * time.Minute // how long the loopback listener waits for the browser
	oauthSkew      = 30              // seconds before expiry a token counts as expired
	oauthBodyLimit = 1 << 20         // answers of the authorization server
)

// oauthClient is the HTTP client of the flow: a metadata fetch cannot hang a request.
var oauthClient = &http.Client{Timeout: 30 * time.Second}

type oauthMeta struct {
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	Registration  string `json:"registration_endpoint"`
}

// oauthTokens is what the vault keeps for one server.
type oauthTokens struct {
	Access   string `json:"access"`
	Refresh  string `json:"refresh,omitempty"`
	Expires  int64  `json:"expires,omitempty"` // unix seconds; 0 = unknown
	Endpoint string `json:"endpoint"`
	ClientID string `json:"clientId"`
}

func oauthVaultKey(root, scope, name string, c ServerConfig) string {
	return "mcp-oauth:" + mcpFingerprint(root, scope, name, ServerConfig{URL: c.URL})
}

func loadTokens(key string) (oauthTokens, bool) {
	var t oauthTokens
	raw, err := config.SecretGet(key)
	if err != nil || raw == "" || json.Unmarshal([]byte(raw), &t) != nil || t.Access == "" {
		return oauthTokens{}, false
	}
	return t, true
}

func storeTokens(key string, t oauthTokens) error {
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return config.SecretSet(key, string(b))
}

func getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := oauthClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, oauthBodyLimit)).Decode(v)
}

// discover finds the authorization server of an MCP endpoint: the resource's metadata names it; a
// server without that document is its own authorization server.
func discover(ctx context.Context, endpoint string) (oauthMeta, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return oauthMeta{}, fmt.Errorf("invalid server address %q", endpoint)
	}
	origin := u.Scheme + "://" + u.Host
	issuer := origin
	var resource struct {
		Servers []string `json:"authorization_servers"`
	}
	if getJSON(ctx, origin+"/.well-known/oauth-protected-resource"+strings.TrimRight(u.Path, "/"), &resource) == nil && len(resource.Servers) > 0 {
		issuer = strings.TrimRight(resource.Servers[0], "/")
	}
	var meta oauthMeta
	for _, doc := range metadataURLs(issuer) {
		if getJSON(ctx, doc, &meta) == nil && meta.Authorization != "" && meta.Token != "" {
			return meta, nil
		}
	}
	return oauthMeta{}, fmt.Errorf("no authorization server metadata at %s", issuer)
}

func metadataURLs(issuer string) []string {
	u, err := url.Parse(issuer)
	if err != nil {
		return nil
	}
	origin := u.Scheme + "://" + u.Host
	tail := strings.TrimRight(u.Path, "/")
	return []string{
		origin + "/.well-known/oauth-authorization-server" + tail,
		origin + "/.well-known/openid-configuration" + tail,
		origin + path.Join(tail, ".well-known/openid-configuration"),
	}
}

// registerClient asks for a client id (a public client: PKCE stands in for a secret).
func registerClient(ctx context.Context, endpoint, redirect string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name": "infuse", "redirect_uris": []string{redirect}, "token_endpoint_auth_method": "none",
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(body)))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := oauthClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, oauthBodyLimit)).Decode(&out); err != nil || out.ClientID == "" {
		return "", fmt.Errorf("client registration failed: HTTP %d", resp.StatusCode)
	}
	return out.ClientID, nil
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("infuse: no system randomness: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// oauthFlow is one login in progress.
type oauthFlow struct {
	meta                                oauthMeta
	clientID, verifier, state, redirect string
	resource, vaultKey                  string
}

// BeginOAuth prepares a login and returns the address the user opens. done is called when the browser
// came back, or the wait ended.
func BeginOAuth(ctx context.Context, root, scope, name string, c ServerConfig, done func(error)) (string, error) {
	if !c.IsHTTP() {
		return "", core.ErrMsg(core.M("mcp.oauth_http_only"))
	}
	meta, err := discover(ctx, c.URL)
	if err != nil {
		return "", err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	f := &oauthFlow{meta: meta, verifier: randomToken(), state: randomToken(), resource: c.URL, vaultKey: oauthVaultKey(root, scope, name, c),
		redirect: fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)}
	if f.clientID, err = oauthClientID(ctx, meta, c, f.redirect); err != nil {
		_ = ln.Close()
		return "", err
	}
	go f.await(ln, done)
	return f.authorizeURL(), nil
}

func oauthClientID(ctx context.Context, meta oauthMeta, c ServerConfig, redirect string) (string, error) {
	if c.ClientID != "" {
		return c.ClientID, nil
	}
	if meta.Registration == "" {
		return "", core.ErrMsg(core.M("mcp.oauth_no_client"))
	}
	return registerClient(ctx, meta.Registration, redirect)
}

func (f *oauthFlow) authorizeURL() string {
	q := url.Values{"response_type": {"code"}, "client_id": {f.clientID}, "redirect_uri": {f.redirect}, "state": {f.state},
		"code_challenge": {pkceChallenge(f.verifier)}, "code_challenge_method": {"S256"}, "resource": {f.resource}}
	sep := "?"
	if strings.Contains(f.meta.Authorization, "?") {
		sep = "&"
	}
	return f.meta.Authorization + sep + q.Encode()
}

// await serves the single redirect. A request with the wrong state is someone else's: it does not end
// the wait.
func (f *oauthFlow) await(ln net.Listener, done func(error)) {
	ctx, cancel := context.WithTimeout(context.Background(), oauthLoginWait)
	defer cancel()
	result := make(chan error, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/callback" || q.Get("state") != f.state {
			http.Error(w, "unknown request", http.StatusBadRequest)
			return
		}
		err := f.finish(ctx, q)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err != nil {
			_, _ = io.WriteString(w, "Sign-in failed: "+err.Error())
		} else {
			_, _ = io.WriteString(w, "Signed in. You can close this window.")
		}
		select {
		case result <- err:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	var err error
	select {
	case err = <-result:
	case <-ctx.Done():
		err = core.ErrMsg(core.M("mcp.oauth_timeout"))
	}
	shut, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	_ = srv.Shutdown(shut)
	if done != nil {
		done(err)
	}
}

func (f *oauthFlow) finish(ctx context.Context, q url.Values) error {
	if e := q.Get("error"); e != "" {
		return fmt.Errorf("%s %s", e, q.Get("error_description"))
	}
	t, err := tokenRequest(ctx, f.meta.Token, f.clientID, url.Values{"grant_type": {"authorization_code"}, "code": {q.Get("code")},
		"redirect_uri": {f.redirect}, "code_verifier": {f.verifier}, "resource": {f.resource}})
	if err != nil {
		return err
	}
	return storeTokens(f.vaultKey, t)
}

// tokenRequest posts to the token endpoint and reads the answer.
func tokenRequest(ctx context.Context, endpoint, clientID string, form url.Values) (oauthTokens, error) {
	form.Set("client_id", clientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return oauthTokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := oauthClient.Do(req)
	if err != nil {
		return oauthTokens{}, err
	}
	defer resp.Body.Close()
	var out struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, oauthBodyLimit)).Decode(&out); err != nil {
		return oauthTokens{}, fmt.Errorf("token endpoint: HTTP %d", resp.StatusCode)
	}
	if out.Access == "" {
		return oauthTokens{}, fmt.Errorf("token endpoint: %s %s", resp.Status, out.Error)
	}
	t := oauthTokens{Access: out.Access, Refresh: out.Refresh, Endpoint: endpoint, ClientID: clientID}
	if out.Expires > 0 {
		t.Expires = time.Now().Unix() + out.Expires
	}
	return t, nil
}

// bearer is the client's current access token, refreshed when it is about to expire ("" = none).
func (c *Client) bearer(ctx context.Context) string {
	key := oauthVaultKey(c.root, c.scope, c.name, c.cfg)
	t, ok := loadTokens(key)
	if !ok {
		return ""
	}
	if t.Expires > 0 && time.Now().Unix() > t.Expires-oauthSkew {
		if fresh, err := c.refreshTokens(ctx, key, t); err == nil {
			return fresh.Access
		}
	}
	return t.Access
}

// refreshTokens trades the refresh token for a new pair; a server may keep the old refresh token.
func (c *Client) refreshTokens(ctx context.Context, key string, t oauthTokens) (oauthTokens, error) {
	if t.Refresh == "" {
		return t, core.ErrMsg(core.M("mcp.oauth_login_needed", "name", c.name))
	}
	fresh, err := tokenRequest(ctx, t.Endpoint, t.ClientID, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {t.Refresh}, "resource": {c.cfg.URL}})
	if err != nil {
		return t, err
	}
	if fresh.Refresh == "" {
		fresh.Refresh = t.Refresh
	}
	return fresh, storeTokens(key, fresh)
}

// retryAuth answers a 401: true when a refresh produced a token worth another attempt.
func (c *Client) retryAuth(ctx context.Context) bool {
	key := oauthVaultKey(c.root, c.scope, c.name, c.cfg)
	t, ok := loadTokens(key)
	if !ok {
		return false
	}
	_, err := c.refreshTokens(ctx, key, t)
	return err == nil
}

// HasOAuthLogin reports whether the vault holds tokens for this server.
func HasOAuthLogin(root, scope, name string, c ServerConfig) bool {
	_, ok := loadTokens(oauthVaultKey(root, scope, name, c))
	return ok
}
