package oracle

// HTTP and browser checks only measure the snapshot when the server they talk to was started from the
// snapshot. The criteria declare that command in "server"; the executor starts it in the snapshot
// folder, contained, waits for every address to answer, verifies that each answering listener belongs
// to the process tree it started, runs every check, and ends the tree.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"infuse/internal/platform/process"
)

// needsServer reports whether any check talks to a server.
func needsServer(s OracleSpec) bool {
	return len(serverURLs(s)) > 0
}

// startServer runs the spec's server in the snapshot and waits for the addresses the checks use. The
// ports must be free, and every answering listener must belong to the child we started: an impostor,
// on any address family and at any moment, is refused instead of measured.
func startServer(ctx context.Context, x containment, evDir string, s OracleSpec) (func(), int, error) {
	urls := serverURLs(s)
	for _, u := range urls {
		if err := portFree(u); err != nil {
			return nil, 0, err
		}
	}
	logPath := filepath.Join(evDir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, 0, err
	}
	cmd := x.command(context.Background(), s.Server)
	cmd.Env = append(cmd.Env, "PORT="+portOf(firstURL(s)))
	cmd.Stdout, cmd.Stderr = logFile, logFile
	process.ProcGroup(cmd)
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, 0, err
	}
	process.AttachJob(cmd)
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	stop := func() {
		process.KillTree(cmd)
		logFile.Close()
	}
	wait := s.ServerSec
	if wait <= 0 {
		wait = 60
	}
	if err := awaitServer(ctx, cmd, urls, exited, wait); err != nil {
		stop()
		return nil, 0, err
	}
	return stop, cmd.Process.Pid, nil
}

// awaitServer returns once every address answers from the started tree, or says why that did not happen.
func awaitServer(ctx context.Context, cmd *exec.Cmd, urls []string, exited <-chan error, waitSec int) error {
	gone := func() error {
		select {
		case err := <-exited:
			return fmt.Errorf("the server exited before answering (%v); see server.log", err)
		default:
			return nil
		}
	}
	deadline := time.Now().Add(time.Duration(waitSec) * time.Second)
	for time.Now().Before(deadline) {
		if err := gone(); err != nil {
			return err
		}
		if allAnswer(ctx, urls) {
			if err := gone(); err != nil {
				return err
			}
			return verifyListeners(urls, cmd)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("the server did not answer within %ds (see server.log)", waitSec)
}

// isLocal reports whether a URL points at this machine, whatever the spelling.
func isLocal(host string) bool {
	if host == "" || strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsUnspecified())
}

// portFree refuses a port something already answers on, or the verdict would measure it instead of the
// snapshot. Both loopback stacks count: localhost resolves to either, and a listener on one is
// invisible to a probe of the other.
func portFree(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil || !isLocal(u.Hostname()) {
		return nil
	}
	port := portOf(rawURL)
	busy := fmt.Errorf("port %s is already in use: refusing to measure a server that is not the snapshot's", port)
	for _, host := range []string{"127.0.0.1", "::1"} {
		if conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 300*time.Millisecond); err == nil {
			_ = conn.Close()
			return busy
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return busy
	}
	return ln.Close()
}

// verifyListeners checks that what answers each local address belongs to the started process tree.
func verifyListeners(urls []string, cmd *exec.Cmd) error {
	for _, raw := range urls {
		u, err := url.Parse(raw)
		if err != nil || !isLocal(u.Hostname()) {
			continue
		}
		port, err := strconv.Atoi(portOf(raw))
		if err != nil {
			return err
		}
		owned, err := process.OwnsListener(cmd, port)
		if err != nil {
			return err
		}
		if !owned {
			return errors.New(uiText("oracle.serverNotOwned", "url", raw))
		}
	}
	return nil
}

// allAnswer reports whether every address answers.
func allAnswer(ctx context.Context, urls []string) bool {
	for _, u := range urls {
		if !serverAnswers(ctx, u) {
			return false
		}
	}
	return true
}

// serverAnswers: any HTTP status means the server is up; a connection error means it is not.
func serverAnswers(ctx context.Context, url string) bool {
	if url == "" {
		return true
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}

// serverURLs lists the addresses the checks read, once each.
func serverURLs(s OracleSpec) []string {
	var out []string
	seen := map[string]bool{}
	for _, c := range s.Checks {
		if (c.Kind == "http" || c.Kind == "browser") && c.URL != "" && !seen[c.URL] {
			seen[c.URL] = true
			out = append(out, c.URL)
		}
	}
	return out
}

func firstURL(s OracleSpec) string {
	if urls := serverURLs(s); len(urls) > 0 {
		return urls[0]
	}
	return ""
}

// portOf extracts the port of a URL for servers that read PORT.
func portOf(url string) string {
	u := strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = u[:i]
	}
	if i := strings.LastIndex(u, ":"); i >= 0 {
		return u[i+1:]
	}
	if strings.HasPrefix(url, "https://") {
		return "443"
	}
	return "80"
}
