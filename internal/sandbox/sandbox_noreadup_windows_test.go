//go:build windows

package sandbox

import (
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A key file labeled no-read-up stays readable by infuse and unreadable by a contained process, with
// and without the label: the "without" half proves the probe itself can read.
func TestContainedProcessCannotReadLabeledFile(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	outside := t.TempDir()
	plain := filepath.Join(outside, "plain.key")
	labeled := filepath.Join(outside, "labeled.key")
	for _, p := range []string{plain, labeled} {
		if err := os.WriteFile(p, []byte("secret-bytes\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := LabelNoReadUp(labeled); err != nil {
		t.Fatal(err)
	}
	if err := LabelNoReadUp(labeled); err != nil { // idempotent
		t.Fatal(err)
	}
	if b, err := os.ReadFile(labeled); err != nil || !strings.Contains(string(b), "secret") {
		t.Fatalf("infuse itself must keep reading the file: %v", err)
	}
	if code := bashRun(t, root, "cat '"+filepath.ToSlash(plain)+"' > got.txt"); code != 0 {
		t.Fatalf("the probe could not read an unlabeled file: exit %d", code)
	}
	if code := bashRun(t, root, "cat '"+filepath.ToSlash(labeled)+"'"); code == 0 {
		t.Fatal("a contained process read a no-read-up file")
	}
}

// Files created in a labeled folder inherit the label, so anchors written later are covered too.
func TestNoReadUpFolderCoversNewFiles(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	root := t.TempDir()
	dir := filepath.Join(t.TempDir(), "keys")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := LabelNoReadUp(dir); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "later.json")
	if err := core.WriteJSON(p, map[string]string{"k": "v"}); err != nil {
		t.Fatal(err)
	}
	if code := bashRun(t, root, "cat '"+filepath.ToSlash(p)+"'"); code == 0 {
		t.Fatal("a contained process read a file inside a no-read-up folder")
	}
}

// noVault answers like a machine without a credential vault.
type noVault struct{}

func (noVault) Get(string) (string, error) { return "", os.ErrPermission }
func (noVault) Set(string, string) error   { return os.ErrPermission }
func (noVault) Delete(string) error        { return os.ErrPermission }

// A signing key written by config lands labeled.
func TestSigningKeyFileIsUnreadableWhenContained(t *testing.T) {
	home := t.TempDir()
	t.Setenv("INFUSE_HOME", home)
	old := config.SecretBackend
	config.SecretBackend = noVault{}
	t.Cleanup(func() { config.SecretBackend = old })
	p := filepath.Join(home, "signing.key")
	if k := config.SigningKey("test-key", p); len(k) < 32 {
		t.Fatal("no key")
	}
	out, err := exec.Command("icacls", p).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "NR") {
		t.Fatalf("the key file has no no-read-up label:\n%s", out)
	}
	if code := bashRun(t, t.TempDir(), "cat '"+filepath.ToSlash(p)+"'"); code == 0 {
		t.Fatal("a contained process read the signing key file")
	}
}
