// Package data holds declarative tables (provider presets, model limits, prices, settings metadata,
// messages and their schemas) embedded in the binary. It is a leaf: no module imports.
package data

import "embed"

//go:embed *.json
var files embed.FS

// Read returns an embedded table. The file set is fixed at build time, so a missing file is a
// programming error, not a runtime condition.
func Read(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		panic("infuse: embedded data file missing: " + name)
	}
	return b
}
