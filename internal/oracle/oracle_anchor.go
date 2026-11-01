package oracle

// Anchoring: the criteria hash is kept in the user's own store outside the project, and every verdict
// carries an HMAC. A criteria file or a verdict edited inside the project no longer verifies, even if
// both project files agree with each other.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"infuse/internal/config"
	"infuse/internal/core"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const hmacSecretName = "oracle-hmac-key"

type oracleAnchor struct {
	Root     string `json:"root"`
	SpecHash string `json:"specHash"`
	Saved    int64  `json:"saved"`
}

func anchorFile(root string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(core.InfuseHome(), "oracle", hex.EncodeToString(sum[:8])+".json")
}

// SaveAnchor records the criteria the user saved in their own store, outside the project.
func SaveAnchor(root string, s OracleSpec) error {
	return core.WriteJSON(anchorFile(root), oracleAnchor{
		Root: filepath.Clean(root), SpecHash: specHash(s), Saved: time.Now().UnixMilli(),
	})
}

// verifyAnchor refuses criteria that differ from the user's saved copy. The first run of a project
// that predates anchoring records the current file; from then on any edit inside the project fails.
func verifyAnchor(root string, s OracleSpec) error {
	var a oracleAnchor
	err := core.ReadJSON(anchorFile(root), &a)
	if errors.Is(err, os.ErrNotExist) {
		return SaveAnchor(root, s)
	}
	if err != nil {
		return err
	}
	if a.SpecHash != specHash(s) {
		return fmt.Errorf("the criteria do not match the copy the user saved (%s)", short(a.SpecHash))
	}
	return nil
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// verdictSigningFile is the fallback key store when the OS vault is unavailable.
func verdictSigningFile() string {
	return filepath.Join(core.InfuseHome(), "oracle-hmac.key")
}

func signingKey() []byte {
	return config.SigningKey(hmacSecretName, verdictSigningFile())
}

// signVerdict signs every field of the verdict except the signature itself.
func signVerdict(v Verdict) string {
	v.Signature = ""
	b, _ := json.Marshal(v)
	mac := hmac.New(sha256.New, signingKey())
	mac.Write(b)
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyVerdict checks a stored verdict's signature.
func verifyVerdict(v Verdict) bool {
	if v.Signature == "" {
		return false
	}
	return hmac.Equal([]byte(signVerdict(v)), []byte(v.Signature))
}
