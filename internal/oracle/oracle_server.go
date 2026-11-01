package oracle

// HTTP checks only measure the snapshot when the server they talk to was started from the snapshot.
// The criteria declare that command in "server"; the executor starts it in the snapshot folder, waits
// for the first address to answer, runs every check, and ends the process tree.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"infuse/internal/core"
	"infuse/internal/platform/process"
)

// hasHTTPChecks reports whether any check needs a server.
func hasHTTPChecks(s OracleSpec) bool {
	for _, c := range s.Checks {
		if c.Kind == "http" {
			return true
		}
	}
	return false
}

// startServer runs the spec's server in the snapshot and waits for the first HTTP check's address.
// The port must be free and the answering listener must belong to the child we started: an impostor
// already on the port is refused instead of measured.
func startServer(ctx context.Context, snapDir, evDir string, s OracleSpec) (func(), int, error) {
	if err := portFree(firstURL(s)); err != nil {
		return nil, 0, err
	}
	logPath := filepath.Join(evDir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, 0, err
	}
	cmd := exec.Command(core.FindRealBash(), "-c", s.Server)
	process.NoConsoleWindow(cmd)
	cmd.Dir = snapDir
	cmd.Env = append(os.Environ(), "CI=1", "INFUSE_ORACLE=1", "PORT="+portOf(firstURL(s)))
	cmd.Stdout, cmd.Stderr = logFile, logFile
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
	pid := cmd.Process.Pid
	wait := s.ServerSec
	if wait <= 0 {
		wait = 60
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-exited:
			logFile.Close()
			return nil, 0, fmt.Errorf("the server exited before answering (%v); see server.log", err)
		default:
		}
		if serverAnswers(ctx, firstURL(s)) {
			select {
			case err := <-exited:
				logFile.Close()
				return nil, 0, fmt.Errorf("the server exited before answering (%v); see server.log", err)
			default:
			}
			return stop, pid, nil
		}
		select {
		case <-ctx.Done():
			stop()
			return nil, 0, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	stop()
	return nil, 0, fmt.Errorf("the server did not answer within %ds (see server.log)", wait)
}

// portFree reserves the check's loopback port for a moment: if another process already answers there,
// the verdict would measure it instead of the snapshot.
func portFree(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	host := u.Hostname()
	if host != "" && host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil
	}
	port := u.Port()
	if port == "" {
		return nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+port)
	if err != nil {
		return fmt.Errorf("port %s is already in use: refusing to measure a server that is not the snapshot's", port)
	}
	return ln.Close()
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

func firstURL(s OracleSpec) string {
	for _, c := range s.Checks {
		if c.Kind == "http" && c.URL != "" {
			return c.URL
		}
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
