package agent

import (
	"infuse/internal/sandbox"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A command that fails must read as failed: the claim checks, the test sentinel, the research gate and
// the circuit breaker all start from this bit.
func TestBashKeepsTheExitStatusOfTheCommand(t *testing.T) {
	testHome(t)
	s, tc, _ := oracleSession(t)
	_ = s
	out, isErr := run(t, tc, "Bash", map[string]any{"command": "false"})
	if !isErr || !strings.Contains(out, "[exit 1]") {
		t.Fatalf("a failing command was reported as success: %v %q", isErr, out)
	}
	out, isErr = run(t, tc, "Bash", map[string]any{"command": "true"})
	if isErr {
		t.Fatalf("a passing command was reported as failed: %q", out)
	}
	out, isErr = run(t, tc, "Bash", map[string]any{"command": "sh -c 'exit 7'"})
	if !isErr || !strings.Contains(out, "[exit 7]") {
		t.Fatalf("the status was not preserved: %v %q", isErr, out)
	}
}

// The parser reads text; the kernel does not care how a path was spelled. A write the guard cannot see
// must still fail, through the same Bash tool the model calls.
func TestContainmentStopsWhatTheParserCannotRead(t *testing.T) {
	if !sandbox.Available() {
		t.Skip("this platform has no OS containment")
	}
	testHome(t)
	_, tc, oracle := oracleSession(t)
	outside := filepath.Join(t.TempDir(), "escape.txt")
	for _, command := range []string{
		`d=.inf; e=use; printf hack >> "$d$e/ora""cle/ready.md"`,
		`eval "$(printf 'echo hack >> .infuse/orac%s/ready.md' le)"`,
		`printf 'echo hack >> .infuse/oracle/ready.md' | sh`,
		`node -e "require('fs').appendFileSync('.inf'+'use/oracle/ready.md','hack')"`,
		`printf hack > '` + filepath.ToSlash(outside) + `'`,
	} {
		run(t, tc, "Bash", map[string]any{"command": command})
	}
	if b, err := os.ReadFile(oracle); err != nil || string(b) != "ORIGINAL\n" {
		t.Fatalf("a hidden write reached the oracle: %q %v", b, err)
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a hidden write left the project")
	}
}

// The working directory is still tracked across a failing command.
func TestBashTracksTheDirectoryAcrossAFailure(t *testing.T) {
	testHome(t)
	s, tc, _ := oracleSession(t)
	if _, isErr := run(t, tc, "Bash", map[string]any{"command": "mkdir -p sub && cd sub && false"}); !isErr {
		t.Fatal("the command must fail")
	}
	if !strings.HasSuffix(strings.ReplaceAll(s.Cwd, `\`, "/"), "/sub") {
		t.Fatalf("cwd: %q", s.Cwd)
	}
}
