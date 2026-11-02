//go:build !windows

package config

// Unix: the fallback signing key is the file itself, protected by its mode (0600) and by living in
// the user's home. There is no per-user key wrapping available without a daemon, so the store keeps
// the raw bytes and marks them as unprotected.

// vaultExposedToContainment is false: reads are not contained here at all, so the vault is the best
// store these platforms have.
const vaultExposedToContainment = false

func protectKey(k []byte) (blob []byte, protected bool, err error) {
	return k, false, nil
}

func unprotectKeyBytes(blob []byte) ([]byte, error) {
	return blob, nil
}
