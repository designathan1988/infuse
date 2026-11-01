package sandbox

// A test binary has no hidden __sandbox mode; wrapping it would re-run the tests. Package tests that
// need the real confinement call the platform core directly (internal/sandbox tests).

import (
	"os"
	"path/filepath"
	"strings"
)

func testBinary() bool {
	if os.Getenv("INFUSE_SANDBOX_FORCE") != "" {
		return false
	}
	base := strings.ToLower(filepath.Base(os.Args[0]))
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
}
