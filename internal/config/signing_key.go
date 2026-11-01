package config

// The signing keys that decide what may run and what is done (approvals, verdicts) live in the OS
// vault. A machine without a vault keeps them in a private file outside the project; where the OS
// offers per-user encryption the file carries protected bytes, not the raw key, so a copied file is
// useless. Keys written by older builds as plain bytes are migrated on first use.

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

const keyFilePrefix = "infuse-key-v1:"

// SigningKey returns at least 32 bytes of key material from the vault entry secretName. When the
// vault is unavailable, fallbackFile (a full path, normally inside the infuse home) holds the key in
// the most protected form the platform offers; a missing key is created.
func SigningKey(secretName, fallbackFile string) []byte {
	if k := keyFromVault(secretName); len(k) >= 32 {
		return k
	}
	if k := keyFromFile(fallbackFile); len(k) >= 32 {
		return k
	}
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	storeKey(secretName, fallbackFile, k)
	return k
}

func keyFromVault(name string) []byte {
	k, err := SecretGet(name)
	if err != nil || len(k) < 32 {
		return nil
	}
	if b, err := hex.DecodeString(strings.TrimSpace(k)); err == nil && len(b) >= 32 {
		return b
	}
	return []byte(k)
}

func keyFromFile(path string) []byte {
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 32 {
		return nil
	}
	k, err := unprotectKey(b)
	if err != nil || len(k) < 32 {
		return nil
	}
	if !isProtectedForm(b) {
		migrateKeyFile(path, k)
	}
	return k
}

func isProtectedForm(b []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(b)), keyFilePrefix)
}

// migrateKeyFile rewrites a plain-bytes key file in the protected form. The raw key stays valid
// while the rewrite runs; a failure leaves the legacy file in place.
func migrateKeyFile(path string, k []byte) {
	blob, protected, err := protectKey(k)
	if err != nil {
		return
	}
	if protected {
		blob = append([]byte(keyFilePrefix), []byte(base64.StdEncoding.EncodeToString(blob))...)
	}
	_ = os.WriteFile(path, blob, 0o600)
}

// storeKey writes the key to the vault first; the protected file is the fallback store for machines
// without one.
func storeKey(secretName, fallbackFile string, k []byte) {
	if err := SecretSet(secretName, hex.EncodeToString(k)); err == nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(fallbackFile), 0o700)
	migrateKeyFile(fallbackFile, k)
}

// unprotectKey reverses storeKey's fallback form. A legacy file holds the raw bytes; the caller
// migrates it by storing the key again.
func unprotectKey(b []byte) ([]byte, error) {
	s := strings.TrimSpace(string(b))
	if raw, ok := strings.CutPrefix(s, keyFilePrefix); ok {
		blob, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, err
		}
		return unprotectKeyBytes(blob)
	}
	return b, nil
}
