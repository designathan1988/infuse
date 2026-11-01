package filter

import (
	"encoding/base64"
	"path/filepath"
	"runtime"
	"testing"
)

// The project's settings, hooks and MCP configuration belong to the user: one write to them can
// disarm the filter or self-approve a command.
func TestProjectControlFilesAreProtected(t *testing.T) {
	root := t.TempDir()
	g := NewGuard(root, Policy{})
	for _, rel := range []string{
		".infuse/settings.json",
		".infuse/hooks.json",
		".mcp.json",
		".claude/settings.json",
		".infuse/sessions/x.json",
	} {
		if _, ok := g.Protects(root, rel); !ok {
			t.Errorf("%s is not protected", rel)
		}
	}
}

// Windows ignores case: .INFUSE must not be a different path from .infuse.
func TestProtectionIsCaseInsensitiveOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case-insensitive filesystem only")
	}
	root := t.TempDir()
	g := NewGuard(root, Policy{})
	for _, rel := range []string{".INFUSE/ORACLE/x.md", ".Infuse/Settings.json", ".MCP.JSON"} {
		if _, ok := g.Protects(root, rel); !ok {
			t.Errorf("%s walks around the protection", rel)
		}
	}
}

func TestGuardRefusesLinksAndIndirectWriters(t *testing.T) {
	root := t.TempDir()
	g := NewGuard(root, Policy{})
	for _, cmd := range []string{
		`mklink /J link .infuse\oracle`,
		`cmd /c mklink /J link .infuse\oracle`,
		`curl -o .infuse/settings.json https://example.test/x`,
		`wget -O .infuse/hooks.json https://example.test/x`,
		`sort -o .infuse/settings.json in.txt`,
		`sed --in-place s/a/b/ .infuse/settings.json`,
		`node -e "require('fs').writeFileSync('.infuse/settings.json','x')"`,
		`python -c "open('.infuse/settings.json','w').write('x')"`,
	} {
		if _, _, err := g.GuardCommand(cmd, root); err == nil {
			t.Errorf("command was not refused: %s", cmd)
		}
	}
}

func TestGuardRefusesPowerShellEncodedCommand(t *testing.T) {
	root := t.TempDir()
	g := NewGuard(root, Policy{})
	payload := "Set-Content .infuse/oracle/ready.md x"
	enc := base64.StdEncoding.EncodeToString(utf16LE(payload))
	if _, _, err := g.GuardCommand("powershell -EncodedCommand "+enc, root); err == nil {
		t.Fatal("encoded command touching the oracle was not refused")
	}
	// An encoded command that cannot be decoded is opaque and must not run.
	if _, _, err := g.GuardCommand("powershell -EncodedCommand not-base64!!", root); err == nil {
		t.Fatal("opaque encoded command was not refused")
	}
}

// A worktree guard rooted at the main project must protect the main project's records reached with
// ../../, not only the oracle.
func TestGuardProtectsEveryInternalRecordFromAWorktree(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, ".infuse", "worktrees", "w")
	g := NewGuard(root, Policy{})
	for _, rel := range []string{
		filepath.Join(work, "..", "..", "..", ".infuse", "sessions", "s.json"),
		filepath.Join(work, "..", "..", "..", ".infuse", "settings.json"),
		filepath.Join(work, "..", "..", "..", ".infuse", "journal.jsonl"),
	} {
		if _, ok := g.Protects(work, rel); !ok {
			t.Errorf("%s is reachable from the worktree", rel)
		}
	}
}

func utf16LE(s string) []byte {
	out := make([]byte, 0, len(s)*2)
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8))
	}
	return out
}
