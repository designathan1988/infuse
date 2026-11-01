package config

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// brokenVault answers every request with an error, like a machine without a credential vault.
type brokenVault struct{}

func (brokenVault) Get(string) (string, error) { return "", os.ErrPermission }
func (brokenVault) Set(string, string) error   { return os.ErrPermission }
func (brokenVault) Delete(string) error        { return os.ErrPermission }

func TestSigningKeyWithoutVaultUsesTheProtectedFile(t *testing.T) {
	testHome(t)
	prev := SecretBackend
	SecretBackend = brokenVault{}
	t.Cleanup(func() { SecretBackend = prev })

	p := filepath.Join(t.TempDir(), "signing.key")
	k1 := SigningKey("test-key", p)
	if len(k1) < 32 {
		t.Fatal("no key returned")
	}
	if k2 := SigningKey("test-key", p); hex.EncodeToString(k1) != hex.EncodeToString(k2) {
		t.Fatal("the key changed between calls")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:len(keyFilePrefix)]) != keyFilePrefix {
		t.Fatalf("the fallback file is not in the protected form: %q", b[:20])
	}
}

func TestLegacyPlainKeyFileIsMigrated(t *testing.T) {
	testHome(t)
	prev := SecretBackend
	SecretBackend = brokenVault{}
	t.Cleanup(func() { SecretBackend = prev })

	p := filepath.Join(t.TempDir(), "signing.key")
	legacy := []byte("0123456789abcdef0123456789abcdef")
	if err := os.WriteFile(p, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	k := SigningKey("test-key", p)
	if string(k) != string(legacy) {
		t.Fatal("the legacy key was not honored")
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b[:len(keyFilePrefix)]) != keyFilePrefix {
		t.Fatal("the legacy file was not migrated to the protected form")
	}
	if k2 := SigningKey("test-key", p); string(k2) != string(legacy) {
		t.Fatal("the migrated file no longer yields the same key")
	}
}

func TestVaultKeyWinsOverTheFile(t *testing.T) {
	testHome(t)
	prev := SecretBackend
	SecretBackend = MemSecrets{"test-key": hex.EncodeToString(make([]byte, 32))}
	t.Cleanup(func() { SecretBackend = prev })

	p := filepath.Join(t.TempDir(), "signing.key")
	if err := os.WriteFile(p, []byte("11111111111111111111111111111111"), 0o600); err != nil {
		t.Fatal(err)
	}
	k := SigningKey("test-key", p)
	for _, b := range k {
		if b != 0 {
			t.Fatal("the vault key was not used")
		}
	}
}
