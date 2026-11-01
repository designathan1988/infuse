// Package ui embeds the window's assets so the binary stays self-contained.
package ui

import "embed"

//go:embed index.html src css vendor
var FS embed.FS
