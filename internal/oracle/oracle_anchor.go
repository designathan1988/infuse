package oracle

// Anchoring: the criteria hash is kept in the user's own store outside the project, and every verdict
// carries an HMAC. A criteria file or a verdict edited inside the project no longer verifies, even if
// both project files agree with each other. The store is labeled no-read-up where the platform has
// labels: the shell the agent runs cannot read it, let alone write it.

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"infuse/internal/config"
	"infuse/internal/core"
	"infuse/internal/sandbox"
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
	Empty    bool   `json:"empty,omitempty"` // the user saved no criteria: the file's absence disarms nothing
}

func anchorStore() string { return filepath.Join(core.InfuseHome(), "oracle") }

// protectStore creates the store and labels it; files created inside inherit the label.
func protectStore() {
	_ = os.MkdirAll(anchorStore(), 0o700)
	_ = sandbox.LabelNoReadUp(anchorStore())
}

func anchorFile(root string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return filepath.Join(anchorStore(), hex.EncodeToString(sum[:8])+".json")
}

// SaveAnchor records the criteria the user saved in their own store, outside the project. Saving also
// accepts the installed dependencies as they are: the next run records a new baseline.
func SaveAnchor(root string, s OracleSpec) error {
	protectStore()
	_ = os.Remove(depsFile(root))
	return core.WriteJSON(anchorFile(root), oracleAnchor{
		Root: filepath.Clean(root), SpecHash: specHash(s), Saved: time.Now().UnixMilli(), Empty: len(s.Checks) == 0,
	})
}

// VerifyCriteria refuses criteria that differ from what the user saved. Only saving creates an anchor:
// criteria with none were not saved by the user, and an anchor without its file means the criteria
// were removed. Both are refused, so deleting the file cannot disarm the oracle.
func VerifyCriteria(root string, s OracleSpec) error {
	var a oracleAnchor
	err := core.ReadJSON(anchorFile(root), &a)
	if errors.Is(err, os.ErrNotExist) {
		if len(s.Checks) == 0 {
			return nil
		}
		return errors.New(uiText("oracle.noAnchor"))
	}
	if err != nil {
		return err
	}
	if !core.Exists(SpecFile(root)) {
		if a.Empty {
			return nil
		}
		return errors.New(uiText("oracle.criteriaRemoved"))
	}
	if a.SpecHash != specHash(s) {
		return errors.New(uiText("oracle.anchor", "error", short(a.SpecHash)))
	}
	return nil
}

// CriteriaAnchored reports whether the criteria on disk are exactly what the user last saved.
func CriteriaAnchored(root string) bool {
	s, err := LoadOracleSpec(root)
	return err == nil && core.Exists(anchorFile(root)) && VerifyCriteria(root, s) == nil
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
	v.SpecVerified = false
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

// projectID binds a verdict to the project it was run for.
func projectID(root string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(root))))
	return hex.EncodeToString(sum[:8])
}

// verdictBelongs checks what the signature alone cannot: a signed verdict is only valid where it was
// produced. An old approved run copied to a newer folder keeps a valid signature but no longer sits
// under its own run name; one copied from another project names another project.
func verdictBelongs(root, runDir string, v Verdict) bool {
	if v.Run != runDir {
		return false
	}
	return v.Project == "" || v.Project == projectID(root) // runs from before the binding have no field
}

// verifyManifest ties the verdict to the tree it measured: the manifest stored with the run must still
// hash to the fingerprint the signature covers.
func verifyManifest(dir, fingerprint string) bool {
	if fingerprint == "" {
		return true // the run ended before a snapshot existed, or predates the binding
	}
	b, err := os.ReadFile(filepath.Join(dir, "snapshot.txt"))
	if err != nil {
		return false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == fingerprint
}
