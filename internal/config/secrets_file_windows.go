//go:build windows

package config

// Secrets on Windows: one DPAPI blob per name under the infuse home, carrying a no-read-up label. The
// agent's contained shell runs as the same user at Low integrity, so neither Credential Manager nor a
// plain private file keeps a provider key from it; the label does, and the blob is useless to another
// user or machine. An entry an older build left in Credential Manager moves here on first read.

import (
	"crypto/sha256"
	"encoding/hex"
	"infuse/internal/core"
	"infuse/internal/platform/fslabel"
	"os"
	"path/filepath"
)

type osSecrets struct{}

func secretFile(name string) string {
	sum := sha256.Sum256([]byte(name))
	return filepath.Join(core.InfuseHome(), "secrets", hex.EncodeToString(sum[:12])+".key")
}

func (osSecrets) Set(name, value string) error {
	blob, _, err := protectKey([]byte(value))
	if err != nil {
		return err
	}
	p := secretFile(name)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	if err := fslabel.NoReadUp(filepath.Dir(p)); err != nil {
		return err
	}
	if err := os.WriteFile(p, blob, 0o600); err != nil {
		return err
	}
	return fslabel.NoReadUp(p)
}

func (s osSecrets) Get(name string) (string, error) {
	if b, err := os.ReadFile(secretFile(name)); err == nil {
		v, err := unprotectKeyBytes(b)
		return string(v), err
	}
	if !vaultIsOurs() {
		return "", nil
	}
	v, err := vaultRead(name)
	if err != nil || v == "" {
		return v, err
	}
	if s.Set(name, v) == nil {
		_ = vaultRemove(name)
	}
	return v, nil
}

func (osSecrets) Delete(name string) error {
	if err := os.Remove(secretFile(name)); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !vaultIsOurs() {
		return nil
	}
	return vaultRemove(name)
}

// The legacy vault, behind seams: tests must prove what would have been read and removed without
// ever touching the user's real Credential Manager.
var (
	vaultRead   = credGet
	vaultRemove = credDelete
)
