// Package data holds declarative tables (provider presets, model limits, prices, settings metadata,
// messages and their schemas) and the starter templates of the bases, embedded in the binary. It is a
// leaf: no module imports.
package data

import "embed"

//go:embed *.json
var files embed.FS

//go:embed all:templates
var templates embed.FS

// Read returns an embedded table. The file set is fixed at build time, so a missing file is a
// programming error, not a runtime condition.
func Read(name string) []byte {
	b, err := files.ReadFile(name)
	if err != nil {
		panic("infuse: embedded data file missing: " + name)
	}
	return b
}

// Template returns one starter template, named relative to internal/data/templates.
func Template(name string) []byte {
	b, err := templates.ReadFile("templates/" + name)
	if err != nil {
		panic("infuse: embedded template missing: " + name)
	}
	return b
}
