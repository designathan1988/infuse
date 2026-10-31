package terminal

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestTerminalEcho(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("no console in CI")
	}
	dir := t.TempDir()
	term, err := Open(dir, dir, 80, 24)
	if err != nil {
		t.Skip("pty unavailable: " + err.Error())
	}
	defer Close(term.ID)
	_, _ = term.Write([]byte("echo infuse-term-$((40+2))\r"))
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(term.Output(), "infuse-term-42") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no echo from the shell; got %q", term.Output())
}
