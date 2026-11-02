package config

// Signed user files: approvals that decide what may run are stored with an HMAC, so editing the file
// inside the project (or from the agent's shell) does not create an approval. The key lives in the OS
// vault, with a private file as the headless fallback.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"infuse/internal/core"
	"os"
	"path/filepath"
)

const signingSecretName = "infuse-signing-key"

// ErrUnsigned means the file is absent or its signature does not verify.
var ErrUnsigned = errors.New("the file is not signed by this user")

type sealedFile struct {
	Data json.RawMessage `json:"data"`
	Sig  string          `json:"sig"`
}

// signingFile is the fallback key store when the OS vault is unavailable.
func signingFile() string { return filepath.Join(core.InfuseHome(), "signing.key") }

func signBytes(b []byte) string {
	mac := hmac.New(sha256.New, SigningKey(signingSecretName, signingFile()))
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil))
}

// SealJSON writes v to path with a signature over its exact JSON.
func SealJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return core.WriteJSON(path, sealedFile{Data: b, Sig: signBytes(b)})
}

// OpenJSON reads a sealed file into v. A missing file is not an error (nothing approved yet); an
// unsigned or tampered file returns ErrUnsigned.
func OpenJSON(path string, v any) error {
	var s sealedFile
	if err := core.ReadJSON(path, &s); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ErrUnsigned
	}
	if err := json.Unmarshal(s.Data, v); err != nil {
		return err
	}
	// WriteJSON indents the document, so the signature is checked over the canonical form of the
	// decoded value rather than over the bytes as they sit on disk.
	canon, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if s.Sig == "" || !hmac.Equal([]byte(s.Sig), []byte(signBytes(canon))) {
		return ErrUnsigned
	}
	return nil
}
