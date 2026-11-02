//go:build windows

package config

import "testing"

// Credential Manager is per Windows user, not per folder. A process on a redirected home must neither
// read the user's real keys nor, through the migration, take them out of the vault.
func TestRedirectedHomeNeverTouchesTheVault(t *testing.T) {
	t.Setenv("INFUSE_HOME", t.TempDir())
	if vaultIsOurs() {
		t.Fatal("a redirected home counts as the user's real one")
	}
	read, removed := 0, 0
	prevRead, prevRemove := vaultRead, vaultRemove
	vaultRead = func(string) (string, error) { read++; return "real-key", nil }
	vaultRemove = func(string) error { removed++; return nil }
	t.Cleanup(func() { vaultRead, vaultRemove = prevRead, prevRemove })
	store := osSecrets{}
	if v, err := store.Get("infuse:deepseek"); err != nil || v != "" {
		t.Fatalf("%q %v", v, err)
	}
	if err := store.Delete("infuse:deepseek"); err != nil {
		t.Fatal(err)
	}
	if read != 0 || removed != 0 {
		t.Fatalf("the real vault was touched: %d reads, %d removals", read, removed)
	}
}

func TestSigningKeyOnARedirectedHomeLeavesTheVaultAlone(t *testing.T) {
	home := t.TempDir()
	t.Setenv("INFUSE_HOME", home)
	removed := 0
	prevRead, prevRemove := vaultRead, vaultRemove
	vaultRead = func(string) (string, error) {
		return "aaaabbbbccccddddeeeeffff0000111122223333444455556666777788889999", nil
	}
	vaultRemove = func(string) error { removed++; return nil }
	t.Cleanup(func() { vaultRead, vaultRemove = prevRead, prevRemove })
	prev := SecretBackend
	SecretBackend = osSecrets{}
	t.Cleanup(func() { SecretBackend = prev })
	if k := SigningKey("infuse-signing-key", home+"/signing.key"); len(k) < 32 {
		t.Fatal("no key")
	}
	if removed != 0 {
		t.Fatal("a redirected home consumed the user's real signing key")
	}
}
