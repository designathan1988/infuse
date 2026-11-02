package llm

// AWS Signature Version 4 for one request, with the standard library alone.
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-create-signed-request.html

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type awsCredentials struct{ access, secret, session string }

const sigV4Algorithm = "AWS4-HMAC-SHA256"

// signV4 adds x-amz-date, the session token when there is one, and the Authorization header.
func signV4(r *http.Request, body []byte, c awsCredentials, region, service string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	day := stamp[:8]
	r.Header.Set("x-amz-date", stamp)
	if c.session != "" {
		r.Header.Set("x-amz-security-token", c.session)
	}
	canonical, signed := canonicalRequest(r, body)
	scope := day + "/" + region + "/" + service + "/aws4_request"
	toSign := strings.Join([]string{sigV4Algorithm, stamp, scope, sha256Hex([]byte(canonical))}, "\n")
	key := hmacSHA256([]byte("AWS4"+c.secret), day)
	for _, part := range []string{region, service, "aws4_request"} {
		key = hmacSHA256(key, part)
	}
	sig := hex.EncodeToString(hmacSHA256(key, toSign))
	r.Header.Set("Authorization", sigV4Algorithm+" Credential="+c.access+"/"+scope+", SignedHeaders="+signed+", Signature="+sig)
}

// canonicalRequest is the text the signature covers, and the list of signed headers.
func canonicalRequest(r *http.Request, body []byte) (text, signed string) {
	values := map[string]string{"host": r.URL.Host}
	names := []string{"host"}
	for k, v := range r.Header {
		name := strings.ToLower(k)
		if name != "authorization" {
			values[name] = strings.TrimSpace(strings.Join(v, ","))
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var headers strings.Builder
	for _, n := range names {
		headers.WriteString(n + ":" + values[n] + "\n")
	}
	signed = strings.Join(names, ";")
	path := r.URL.EscapedPath()
	if path == "" {
		path = "/"
	}
	return strings.Join([]string{r.Method, canonicalPath(path), canonicalQuery(r.URL), headers.String(), signed, sha256Hex(body)}, "\n"), signed
}

// canonicalPath encodes each segment once more, as every service except S3 requires.
func canonicalPath(escaped string) string {
	parts := strings.Split(escaped, "/")
	for i, p := range parts {
		parts[i] = strings.ReplaceAll(url.PathEscape(p), "+", "%20")
	}
	return strings.Join(parts, "/")
}

func canonicalQuery(u *url.URL) string {
	q := u.Query()
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out []string
	for _, k := range keys {
		vs := append([]string{}, q[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			out = append(out, awsEscape(k)+"="+awsEscape(v))
		}
	}
	return strings.Join(out, "&")
}

func awsEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, text string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(text))
	return m.Sum(nil)
}
