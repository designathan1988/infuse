package lang

import "regexp"

// TestPath recognizes a test file by its folder or its name, in every language the map reads.
var TestPath = regexp.MustCompile(`(?i)(^|/)(tests?|__tests__|specs?|e2e)/|[._-](test|spec)\.[a-z0-9]+$|_test\.go$|(^|/)test_[^/]+\.py$|(^|/)conftest\.py$`)

// IsTestPath reports whether a posix path names a test file.
func IsTestPath(rel string) bool { return TestPath.MatchString(rel) }
