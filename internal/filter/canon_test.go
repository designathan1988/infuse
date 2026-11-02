package filter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func windowsOnly(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("Windows path spellings")
	}
}

// Every spelling of one file is that file: this is what each gate compares.
func TestCanonPathFoldsWindowsSpellings(t *testing.T) {
	windowsOnly(t)
	cwd := `C:\proj`
	for in, want := range map[string]string{
		`.mcp.json.`:                               `C:\proj\.mcp.json`,
		`.infuse/hooks.json `:                      `C:\proj\.infuse\hooks.json`,
		`.infuse/hooks.json. .`:                    `C:\proj\.infuse\hooks.json`,
		`.mcp.json::$DATA`:                         `C:\proj\.mcp.json`,
		`.mcp.json:evil`:                           `C:\proj\.mcp.json`,
		`.infuse::$INDEX_ALLOCATION/oracle/x`:      `C:\proj\.infuse\oracle\x`,
		`.infuse:$I30:$INDEX_ALLOCATION/oracle/x`:  `C:\proj\.infuse\oracle\x`,
		`.infuse./evidence/x`:                      `C:\proj\.infuse\evidence\x`,
		`.infuse /evidence/x`:                      `C:\proj\.infuse\evidence\x`,
		`\\?\C:\proj\.infuse\oracle\x`:             `C:\proj\.infuse\oracle\x`,
		`\\.\C:\proj\.infuse\oracle\x`:             `C:\proj\.infuse\oracle\x`,
		`\??\C:\proj\.infuse\oracle\x`:             `C:\proj\.infuse\oracle\x`,
		`//?/C:/proj/.infuse/oracle/x`:             `C:\proj\.infuse\oracle\x`,
		`\\localhost\c$\proj\.infuse\oracle\x`:     `c:\proj\.infuse\oracle\x`,
		`\\127.0.0.1\C$\proj\.infuse\oracle\x`:     `C:\proj\.infuse\oracle\x`,
		`\\?\UNC\localhost\c$\proj\.infuse\oracle`: `c:\proj\.infuse\oracle`,
		`C:.infuse\oracle\x`:                       `C:\proj\.infuse\oracle\x`,
		`/c/proj/x`:                                `C:\proj\x`,
		`/tmp/x`:                                   `C:\tmp\x`,
		`a/../.. /x`:                               `C:\x`,
	} {
		if got := CanonPath(cwd, in); !strings.EqualFold(got, want) {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

// A share or a device that is not this machine's drive stays what it is: not a path in the project.
func TestCanonPathKeepsForeignSharesOutside(t *testing.T) {
	windowsOnly(t)
	for _, p := range []string{`\\server\share\.infuse\oracle\x`, `\\?\UNC\server\share\x`, `\\?\GLOBALROOT\Device\HarddiskVolume3\proj\x`, `\\?\Volume{1}\proj\x`} {
		if got := CanonPath(`C:\proj`, p); strings.HasPrefix(strings.ToLower(got), `c:\proj`) {
			t.Errorf("%q resolved inside the project: %q", p, got)
		}
	}
}

func TestGuardProtectsEveryWindowsSpelling(t *testing.T) {
	windowsOnly(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755); err != nil {
		t.Fatal(err)
	}
	g := newGuard(root, []string{"secrets/**"})
	for _, p := range []string{
		`.mcp.json.`, `.mcp.json `, `.mcp.json::$DATA`, `.mcp.json:ads`, `.infuse/hooks.json `,
		`.infuse./evidence/x`, `.infuse::$INDEX_ALLOCATION/oracle/x`, `.infuse/oracle::$INDEX_ALLOCATION/x`,
		`secrets./key`, `secrets::$INDEX_ALLOCATION/key`,
		`\\?\` + root + `\.infuse\oracle\x`, `\\.\` + root + `\.mcp.json`,
		`\\?\GLOBALROOT\Device\HarddiskVolume3\p\.infuse\oracle\x`, `\\server\share\p\.infuse\oracle\x`,
	} {
		if _, ok := g.Protects(root, p); !ok {
			t.Errorf("%q walks around the protection", p)
		}
	}
	if _, ok := g.Protects(root, "src/main.go"); ok {
		t.Fatal("an ordinary file became protected")
	}
}

// The short name of an existing folder is expanded from the disk, even under a file that does not exist.
func TestGuardSeesShortNames(t *testing.T) {
	windowsOnly(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".infuse", "oracle"), 0o755); err != nil {
		t.Fatal(err)
	}
	short := shortName(t, filepath.Join(root, ".infuse"))
	g := newGuard(root, nil)
	if _, ok := g.Protects(root, short+"/oracle/new.md"); !ok {
		t.Fatalf("%s/oracle/new.md is not protected", short)
	}
}

// shortName reads the 8.3 name from the listing; a volume without short names skips the test.
func shortName(t *testing.T, p string) string {
	t.Helper()
	got := short8dot3(p)
	if got == "" || strings.EqualFold(got, filepath.Base(p)) {
		t.Skip("no 8.3 name on this volume")
	}
	return got
}

// The deepest existing ancestor counts, however deep the part that does not exist yet.
func TestSymlinkResolvedAtTheDeepestAncestor(t *testing.T) {
	root := t.TempDir()
	oracle := filepath.Join(root, ".infuse", "oracle")
	if err := os.MkdirAll(oracle, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(oracle, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks not available: " + err.Error())
	}
	g := newGuard(root, nil)
	for _, p := range []string{"link/x.md", "link/a/b/c/x.md"} {
		if _, ok := g.Protects(root, p); !ok {
			t.Errorf("%s reaches the oracle", p)
		}
	}
}

func TestDanglingLinkIsJudgedAtItsTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, ".infuse", "oracle", "new.md")
	if err := os.Symlink(target, filepath.Join(root, "link")); err != nil {
		t.Skip("symlinks not available: " + err.Error())
	}
	if _, ok := newGuard(root, nil).Protects(root, "link"); !ok {
		t.Fatal("a dangling link into the oracle was not followed")
	}
}

func TestLinkLoopEnds(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "b"), filepath.Join(root, "a")); err != nil {
		t.Skip("symlinks not available: " + err.Error())
	}
	if err := os.Symlink(filepath.Join(root, "a"), filepath.Join(root, "b")); err != nil {
		t.Fatal(err)
	}
	_ = RealPath(filepath.Join(root, "a", "x"))
}
