package oracle

// Run metadata: the criteria a verdict used are hashed and copied beside the evidence, and every
// limit of the run is stated with the verdict instead of being hidden.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func specHash(s OracleSpec) string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeSpecCopy stores the exact criteria used by a run.
func writeSpecCopy(runDir string, s OracleSpec) {
	if b, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(runDir, "checks.json"), b, 0o644)
	}
}

// verifySpecCopy recomputes the hash of the criteria stored with a run.
func verifySpecCopy(runDir, want string) bool {
	b, err := os.ReadFile(filepath.Join(runDir, "checks.json"))
	if err != nil || want == "" {
		return false
	}
	var s OracleSpec
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	return specHash(s) == want
}

// specWarnings states what a run cannot prove: an HTTP check reads whatever server answers the URL,
// so it only measures the snapshot when the server is started from the snapshot itself.
func specWarnings(s OracleSpec) []string {
	if strings.TrimSpace(s.Server) != "" {
		return nil // the executor started the server in the snapshot
	}
	var out []string
	for _, c := range s.Checks {
		if c.Kind == "http" || c.Kind == "browser" {
			out = append(out, fmt.Sprintf("%s check %s reads %s: it measures the server that answers that address, not the snapshot. Declare \"server\" in the criteria so the executor starts the server from the snapshot", c.Kind, c.ID, c.URL))
		}
	}
	return out
}
