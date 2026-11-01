// Package security holds the checks applied to data crossing the application boundary: credential
// redaction, the private-network refusal and external-content provenance.
package security

import (
	"fmt"
	"infuse/internal/core"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

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

// RedactSecrets replaces credentials in text that leaves the tool (to the model, the journal, the UI).
func RedactSecrets(s string) string {
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

// PrivateTarget reports whether a URL points at this machine or the private network (SSRF guard), with
// the reason in English.
func PrivateTarget(raw string) (bool, string) {
	bad, why := PrivateTargetMsg(raw)
	return bad, why.String()
}

// PrivateTargetMsg reports whether a URL points at this machine or the private network, and why.
func PrivateTargetMsg(raw string) (bool, core.Msg) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return true, core.M("safety.bad_url")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return true, core.M("safety.scheme", "scheme", u.Scheme)
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") || strings.HasSuffix(strings.ToLower(host), ".local") || strings.HasSuffix(strings.ToLower(host), ".internal") {
		return true, core.M("safety.local", "host", host)
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return false, core.Msg{}
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.Equal(net.ParseIP("169.254.169.254")) {
			return true, core.M("safety.internal_ip", "ip", ip.String())
		}
	}
	return false, core.Msg{}
}

// BrowserTargetMsg reports whether the agent's browser must refuse a URL. The browser may open the
// web and the project's own local servers (the product exists to test them), but not non-web
// schemes, file:// URLs, or link-local/metadata addresses.
func BrowserTargetMsg(raw string) (bool, core.Msg) {
	u, err := url.Parse(raw)
	if err != nil {
		return true, core.M("safety.bad_url")
	}
	if strings.EqualFold(u.Scheme, "about") {
		return false, core.Msg{}
	}
	if u.Hostname() == "" {
		return true, core.M("safety.bad_url")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return true, core.M("safety.scheme", "scheme", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "169.254.169.254" || strings.HasPrefix(host, "169.254.") || strings.HasSuffix(host, ".internal") {
		return true, core.M("safety.local", "host", host)
	}
	return false, core.Msg{}
}

// ExternalContent labels fetched text as data from a source, never instructions (prompt-injection
// hygiene).
func ExternalContent(source, text string) string {
	return fmt.Sprintf("<external-content source=%q fetched=%q>\n%s\n</external-content>\n", source, time.Now().Format(time.RFC3339), text) + core.M("safety.external").String()
}
